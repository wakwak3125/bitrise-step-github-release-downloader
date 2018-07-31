.PHONY: test
test:
	./scripts/run.sh

.PHONY: clean
clean:
	@rm -rf _tmp

.PHONY: deploy
	./scripts/deploy.sh
