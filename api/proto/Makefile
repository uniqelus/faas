BREAKING_BRANCH ?= origin/dev
# Literal '#' for a buf git input ref; isolated so Make does not treat it as a comment.
HASH := \#

generate:
	buf dep update
	buf generate

lint:
	buf lint

breaking:
	buf breaking --against "../../.git$(HASH)branch=$(BREAKING_BRANCH),subdir=api/proto"

.PHONY: generate lint breaking
