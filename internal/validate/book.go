package validate

func bookRules(recordID string, data object) *Failure {
	contentHash, f := data.text("contentHash")
	if f != nil {
		return f
	}
	if !syncKeyPattern.MatchString(contentHash) {
		return fail(ReasonBadPattern, data.at("contentHash"))
	}
	if f := bookFieldRules(data); f != nil {
		return f
	}
	if contentHash != recordID {
		return fail(ReasonContentHashDiffersFromKey, data.at("contentHash"))
	}
	return nil
}

func bookFieldRules(data object) *Failure {
	required := []struct {
		name string
		rule check
	}{
		{"fileName", isText},
		{"title", isText},
		{"alias", orNull(isText)},
		{"language", oneOf("ja", "ko", "en")},
		{"direction", oneOf("rtl", "ltr")},
		{"layoutKind", oneOf("paged", "continuous", "flow")},
		{"sourceKind", oneOf("images", "pdf", "archive", "epub")},
		{"imageCount", isCount},
	}
	for _, field := range required {
		if f := data.require(field.name, field.rule); f != nil {
			return f
		}
	}
	if f := data.optional("seriesId", orNull(isNonEmptyText)); f != nil {
		return f
	}
	if f := data.optional("volume", orNull(isNumber)); f != nil {
		return f
	}
	if !data.has("origin") {
		return nil
	}
	origin, f := data.object("origin")
	if f != nil {
		return f
	}
	return bookOriginRules(origin)
}

func bookOriginRules(origin object) *Failure {
	if f := origin.require("catalogId", matching(uuidPattern)); f != nil {
		return f
	}
	if f := origin.require("entryId", isNonEmptyText); f != nil {
		return f
	}
	acquisition, f := origin.object("acquisition")
	if f != nil {
		return f
	}
	if f := acquisitionRules(acquisition); f != nil {
		return f
	}
	if f := origin.require("updated", isText); f != nil {
		return f
	}
	feedPath, f := origin.array("feedPath")
	if f != nil {
		return f
	}
	if f := eachItem(feedPath, origin.at("feedPath"), feedStepRules); f != nil {
		return f
	}
	if f := origin.require("feedPosition", isCount); f != nil {
		return f
	}
	return origin.require("downloadedAt", isNumber)
}

func acquisitionRules(acquisition object) *Failure {
	if f := acquisition.require("href", isNonEmptyText); f != nil {
		return f
	}
	if f := acquisition.require("format", oneOf("epub", "pdf", "cbz")); f != nil {
		return f
	}
	if f := acquisition.require("mediaType", isText); f != nil {
		return f
	}
	return acquisition.require("length", orNull(isCount))
}

func feedStepRules(value any, path string) *Failure {
	step, f := newObject(value, path)
	if f != nil {
		return f
	}
	if f := step.require("title", isText); f != nil {
		return f
	}
	return step.require("href", isText)
}
