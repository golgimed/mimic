FROM golang:1.26.10-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=${VERSION}" -o /mimic ./cmd/mimic
RUN mkdir -p /data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /mimic /mimic
COPY --from=build --chown=nonroot:nonroot /data /data

EXPOSE 3000
USER nonroot:nonroot
ENTRYPOINT ["/mimic"]
