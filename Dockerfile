FROM node:22-alpine AS web-build
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
COPY design/site.css /src/design/site.css
RUN npm run build

FROM golang:1.25-alpine AS go-build
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web-build /src/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X github.com/wyrd-company/lore/internal/version.Value=${VERSION}" -o /out/lore-server ./cmd/lore-server && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X github.com/wyrd-company/lore/internal/version.Value=${VERSION}" -o /out/lore ./cmd/lore

FROM alpine:3.22 AS server
RUN apk add --no-cache ca-certificates
COPY --from=go-build /out/lore-server /usr/local/bin/lore-server
COPY --from=go-build /out/lore /usr/local/bin/lore
EXPOSE 8080/tcp
ENTRYPOINT ["/usr/local/bin/lore-server"]
CMD ["serve"]
