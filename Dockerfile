FROM golang:alpine as build

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -v -o ./app 

FROM alpine
WORKDIR /app
COPY app.conf .
COPY --from=build /app/app /app/app

CMD ["app"]
