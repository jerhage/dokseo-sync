package validate

var tagColours = []string{
	"slate", "clay", "sage", "plum", "rose", "ice", "ruby", "copper",
	"olive", "fern", "cyan", "sky", "indigo", "violet", "magenta", "stone",
}

func tagRules(_ string, data object) *Failure {
	if f := data.require("name", isFilledText); f != nil {
		return f
	}
	if f := data.require("colour", oneOf(tagColours...)); f != nil {
		return f
	}
	return data.require("createdAt", isNumber)
}
