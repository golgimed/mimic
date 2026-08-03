FROM golang:1.26-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /mimic ./cmd/mimic

FROM gcr.io/distroless/static-debian12
COPY --from=build /mimic /mimic

EXPOSE 3000
HEALTHCHECK CMD ["/mimic", "healthcheck"]
ENTRYPOINT ["/mimic"]
