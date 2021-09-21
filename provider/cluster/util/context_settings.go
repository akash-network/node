package util

import "context"

func ApplyToContext(ctx context.Context, config map[interface{}]interface{}) context.Context {
	for k, v := range config {
		ctx = context.WithValue(ctx, k, v)
	}

	return ctx
}
