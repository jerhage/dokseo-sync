package validate

func positionRules(_ string, data object) *Failure {
	kind, f := data.text("kind")
	if f != nil {
		return f
	}
	if f := data.optional("lastReadAt", orNull(isNumber)); f != nil {
		return f
	}
	if f := data.optional("finishedAt", orNull(isNumber)); f != nil {
		return f
	}
	switch kind {
	case "image":
		return imagePositionRules(data)
	case "text":
		return textPositionRules(data)
	default:
		return fail(ReasonUnknownVariant, data.at("kind"))
	}
}

func imagePositionRules(data object) *Failure {
	if f := data.require("index", isCount); f != nil {
		return f
	}
	if f := data.require("shownThrough", isCount); f != nil {
		return f
	}
	return data.require("offset", isFraction)
}

func textPositionRules(data object) *Failure {
	if f := data.require("cfi", isText); f != nil {
		return f
	}
	return data.require("fraction", orNull(isFraction))
}
