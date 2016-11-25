FROM golang:1.6.0

ENV PROJECT_PATH=/volume
ENV GOPATH=/volume
ENV GOBIN=/volume/bin
#COPY . $PROJECT_PATH
WORKDIR $PROJECT_PATH

#RUN go get -v When on production When on production When on production When on production

CMD ["go", "run", "main.go"]
