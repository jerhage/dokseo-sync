package validate

func captureRules(_ string, data object) *Failure {
	if f := data.require("bookKey", matching(syncKeyPattern)); f != nil {
		return f
	}
	anchor, f := data.object("anchor")
	if f != nil {
		return f
	}
	if f := anchorRules(anchor); f != nil {
		return f
	}
	if f := data.require("text", isText); f != nil {
		return f
	}
	origin, f := data.text("origin")
	if f != nil {
		return f
	}
	if f := captureOriginRules(origin, data); f != nil {
		return f
	}
	if f := captureTimeRules(data); f != nil {
		return f
	}
	return captureTagRules(data)
}

func captureOriginRules(origin string, data object) *Failure {
	switch origin {
	case "written":
		if f := data.forbid("note"); f != nil {
			return f
		}
		return data.forbid("confidence")
	case "lifted":
		if f := data.require("note", orNull(isText)); f != nil {
			return f
		}
		return data.forbid("confidence")
	case "recognized":
		if f := data.require("note", orNull(isText)); f != nil {
			return f
		}
		return data.require("confidence", orNull(isNumber))
	default:
		return fail(ReasonNotAllowedValue, data.at("origin"))
	}
}

func captureTimeRules(data object) *Failure {
	if f := data.require("createdAt", isNumber); f != nil {
		return f
	}
	return data.require("editedAt", orNull(isNumber))
}

func captureTagRules(data object) *Failure {
	tagIDs, f := data.array("tagIds")
	if f != nil {
		return f
	}
	if f := eachItem(tagIDs, data.at("tagIds"), matching(uuidPattern)); f != nil {
		return f
	}
	seen := make(map[any]bool, len(tagIDs))
	for position, tagID := range tagIDs {
		if seen[tagID] {
			return fail(ReasonRepeatedItem, itemPath(data.at("tagIds"), position))
		}
		seen[tagID] = true
	}
	return nil
}

func anchorRules(anchor object) *Failure {
	kind, f := anchor.text("kind")
	if f != nil {
		return f
	}
	switch kind {
	case "region":
		return regionAnchorRules(anchor)
	case "text":
		return textAnchorRules(anchor)
	default:
		return fail(ReasonUnknownVariant, anchor.at("kind"))
	}
}

func regionAnchorRules(anchor object) *Failure {
	regions, f := anchor.nonEmptyArray("regions")
	if f != nil {
		return f
	}
	return eachItem(regions, anchor.at("regions"), regionRules)
}

func regionRules(value any, path string) *Failure {
	region, f := newObject(value, path)
	if f != nil {
		return f
	}
	if f := region.require("index", isCount); f != nil {
		return f
	}
	rect, f := region.object("rect")
	if f != nil {
		return f
	}
	return rectRules(rect)
}

func rectRules(rect object) *Failure {
	if f := rect.require("x", isFraction); f != nil {
		return f
	}
	if f := rect.require("y", isFraction); f != nil {
		return f
	}
	if f := rect.require("width", isPositiveFraction); f != nil {
		return f
	}
	return rect.require("height", isPositiveFraction)
}

func textAnchorRules(anchor object) *Failure {
	if f := anchor.require("cfi", isNonEmptyText); f != nil {
		return f
	}
	quote, f := anchor.object("quote")
	if f != nil {
		return f
	}
	if f := quote.require("exact", isNonEmptyText); f != nil {
		return f
	}
	if f := quote.require("prefix", isText); f != nil {
		return f
	}
	if f := quote.require("suffix", isText); f != nil {
		return f
	}
	return anchor.require("chapter", orNull(isNonEmptyText))
}
