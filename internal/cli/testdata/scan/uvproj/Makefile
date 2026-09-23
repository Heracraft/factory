test:
	uv run pytest
lint:
	ruff check . && mypy src
	pre-commit run -a
serve:
	svc serve --reload
