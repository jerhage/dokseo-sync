package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/jerhage/dokseo-sync/internal/store"
	"github.com/jerhage/dokseo-sync/internal/validate"
)

const (
	maxPushChanges    = 500
	defaultPullLimit  = 500
	maxPullLimit      = 1000
	invalidRequestMsg = "the request is malformed"
)

type pushRequest struct {
	Changes []json.RawMessage `json:"changes"`
}

type acceptedChange struct {
	ID  string `json:"id"`
	Seq int64  `json:"seq"`
}

type pushResponse struct {
	Accepted []acceptedChange `json:"accepted"`
}

type pulledChange struct {
	ID         string          `json:"id"`
	Kind       string          `json:"kind"`
	RecordID   string          `json:"recordId"`
	Data       json.RawMessage `json:"data"`
	UpdatedAt  int64           `json:"updatedAt"`
	DeviceID   string          `json:"deviceId"`
	Seq        int64           `json:"seq"`
	ReceivedAt int64           `json:"receivedAt"`
}

type pullResponse struct {
	Changes []pulledChange `json:"changes"`
	Next    int64          `json:"next"`
	More    bool           `json:"more"`
}

func (s *server) pushChanges(w http.ResponseWriter, r *http.Request) {
	var request pushRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		s.writeError(w, http.StatusBadRequest, codeInvalidRequest, invalidRequestMsg)
		return
	}
	if len(request.Changes) < 1 || len(request.Changes) > maxPushChanges {
		s.writeError(w, http.StatusBadRequest, codeInvalidRequest, "changes must hold 1 to 500 entries")
		return
	}
	device := deviceFrom(r)
	for _, raw := range request.Changes {
		var sender struct {
			DeviceID string `json:"deviceId"`
		}
		if json.Unmarshal(raw, &sender) == nil && sender.DeviceID != "" && sender.DeviceID != device.ID {
			s.writeError(w, http.StatusForbidden, codeDeviceMismatch, "the deviceid of a change is not the device of the token")
			return
		}
	}
	now := s.now().UnixMilli()
	changes := make([]store.NewChange, 0, len(request.Changes))
	for i, raw := range request.Changes {
		change, failure := validate.Validate(raw, now)
		if failure != nil {
			s.writeInvalidChange(w, i, failure.Message())
			return
		}
		changes = append(changes, store.NewChange(change))
	}
	seqs, err := s.store.AppendChanges(r.Context(), s.accountID, now, changes)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	accepted := make([]acceptedChange, len(changes))
	for i, change := range changes {
		accepted[i] = acceptedChange{ID: change.ID, Seq: seqs[i]}
	}
	s.writeJSON(w, http.StatusOK, pushResponse{Accepted: accepted})
}

func (s *server) pullChanges(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	after, ok := queryInt(query.Get("after"), query.Has("after"), 0, 0, 1<<62)
	if !ok {
		s.writeError(w, http.StatusBadRequest, codeInvalidRequest, "after must be an integer of 0 or more")
		return
	}
	limit, ok := queryInt(query.Get("limit"), query.Has("limit"), defaultPullLimit, 1, maxPullLimit)
	if !ok {
		s.writeError(w, http.StatusBadRequest, codeInvalidRequest, "limit must be an integer from 1 to 1000")
		return
	}
	stored, more, err := s.store.ChangesAfter(r.Context(), s.accountID, after, int(limit))
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	changes := make([]pulledChange, len(stored))
	next := after
	for i, change := range stored {
		changes[i] = pulledChange{
			ID:         change.ID,
			Kind:       change.Kind,
			RecordID:   change.RecordID,
			Data:       change.Data,
			UpdatedAt:  change.UpdatedAt,
			DeviceID:   change.DeviceID,
			Seq:        change.Seq,
			ReceivedAt: change.ReceivedAt,
		}
		next = change.Seq
	}
	s.writeJSON(w, http.StatusOK, pullResponse{Changes: changes, Next: next, More: more})
}

func queryInt(value string, present bool, fallback, min, max int64) (int64, bool) {
	if !present {
		return fallback, true
	}
	number, err := strconv.ParseInt(value, 10, 64)
	if err != nil || number < min || number > max {
		return 0, false
	}
	return number, true
}
