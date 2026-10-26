package validate

func catalogRules(_ string, data object) *Failure {
	if f := data.require("title", isNonEmptyText); f != nil {
		return f
	}
	if f := data.require("protocol", oneOf("opds1")); f != nil {
		return f
	}
	if f := data.require("rootUrl", isNonEmptyText); f != nil {
		return f
	}
	if f := data.forbid("password"); f != nil {
		return f
	}
	auth, f := data.object("auth")
	if f != nil {
		return f
	}
	return catalogAuthRules(auth)
}

func catalogAuthRules(auth object) *Failure {
	kind, f := auth.text("kind")
	if f != nil {
		return f
	}
	if f := auth.forbid("password"); f != nil {
		return f
	}
	switch kind {
	case "none":
		return nil
	case "basic":
		return auth.require("username", isNonEmptyText)
	default:
		return fail(ReasonUnknownVariant, auth.at("kind"))
	}
}
