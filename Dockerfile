FROM golang:1.22-bookworm AS cli-build

WORKDIR /src
COPY go.mod ./
COPY cmd/apifox-cli/ ./cmd/apifox-cli/
COPY internal/ ./internal/
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/apifox-cli ./cmd/apifox-cli

FROM python:3.12-slim

ENV PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PIP_NO_CACHE_DIR=1 \
    APIFOX_CLI_PATH=/usr/local/bin/apifox-cli \
    APIFOX_MCP_WRITE_MODE=plan

WORKDIR /app
COPY --from=cli-build /out/apifox-cli /usr/local/bin/apifox-cli
COPY pyproject.toml README.md hatch_build.py ./
COPY apifox_mcp/ ./apifox_mcp/
RUN pip install --no-cache-dir . \
    && addgroup --system apifox \
    && adduser --system --ingroup apifox --home /app apifox

USER apifox
ENTRYPOINT ["apifox-mcp"]
