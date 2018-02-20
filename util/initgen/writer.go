package initgen

import "fmt"

type Type string

const (
	TypeDirectory Type = "dir"
	TypeHelm           = "helm"
)

type Writer interface {
	Write() error
}

func CreateWriter(type_ Type, ctx Context) (Writer, error) {
	switch type_ {
	case TypeDirectory:
		if len(ctx.PrivateValidators()) > 1 {
			return NewMultiDirWriter(ctx), nil
		}
		return NewDirWriter(ctx), nil
	case TypeHelm:
		if len(ctx.PrivateValidators()) > 1 {
			return NewMultiHelmWriter(ctx), nil
		}
		return NewHelmWriter(ctx), nil
	}
	return nil, fmt.Errorf("Unknown type: %v", type_)
}
