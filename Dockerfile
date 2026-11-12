# --- Build -------------------------------------------------------------------
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

# --- Run ---------------------------------------------------------------------
FROM alpine:3.20
# poppler-utils provides pdftotext (fast, free PDF text extraction).
# Scanned PDFs that have no text layer are read by Gemini instead.
RUN apk add --no-cache ca-certificates poppler-utils && \
    adduser -D -u 10001 app && mkdir -p /var/data && chown app /var/data
COPY --from=build /out/server /usr/local/bin/server
USER app
ENV PORT=8080 STORAGE_DIR=/var/data/files
EXPOSE 8080
CMD ["server"]
