GOBIN ?= $(HOME)/bin

.PHONY: install
install:
	GOBIN=$(GOBIN) go install ./apps/ws
