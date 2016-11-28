FROM golang
MAINTAINER Sylvain Laurent

ENV GOBIN $GOPATH/bin

ENV PROJECT_PATH /go/src/heatmap

ADD . $PROJECT_PATH/

WORKDIR $PROJECT_PATH

#RUN go get -v

RUN go install

EXPOSE 8080
ENTRYPOINT /go/bin/heatmap
