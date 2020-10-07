gofmt:
	find . -not -path './vendor*' -name '*.go' -type f | xargs gofmt -s -w
