FROM golang:1.22-alpine AS build
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/forgereview ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates curl
RUN addgroup -S forgereview && adduser -S forgereview -G forgereview
WORKDIR /app
COPY --from=build /out/forgereview ./forgereview
RUN mkdir /data && chown forgereview:forgereview /data
USER forgereview
EXPOSE 8088
CMD ["./forgereview"]
