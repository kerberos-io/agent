FROM golang:1.24-bookworm AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /frame-processor .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /frame-processor /frame-processor
EXPOSE 8080
ENTRYPOINT ["/frame-processor"]