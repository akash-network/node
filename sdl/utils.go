package sdl

const (
	valueFalse = "false"
	valueTrue  = "true"
)

// as per yaml following allowed as bool values
func unifyStringAsBool(val string) (string, bool) {
	if val == valueTrue || val == "on" || val == "yes" {
		return valueTrue, true
	} else if val == valueFalse || val == "off" || val == "no" {
		return valueFalse, true
	}

	return "", false
}
