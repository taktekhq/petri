.PHONY: install before after

install:
	docker compose run --rm app bash -c "python -m venv .venv && .venv/bin/pip install -q -r requirements.txt"

before:
	docker compose run --rm app bash -c ".venv/bin/python -m app.migrate && .venv/bin/pytest -v"

after:
	docker compose run --rm -e PORT=5433 app bash -c ".venv/bin/python -m app.migrate && .venv/bin/pytest -v"
