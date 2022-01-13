# broadcast

A simple Go server that broadcasts any data/stream. Read more about it [on my blog](https://schollz.com/blog/radio/). The code was written based of [schollz/duct](https://github.com/schollz/duct), which is a fork of of [patchbay-pub](https://github.com/patchbay-pub/patchbay-simple-server).


## Usage

### Sending data

You can POST data. 

```
curl -X POST --data-binary "@someimage.png" localhost:9222/test.png
```

The image will be available at `localhost:9222/test.png`.


### Streaming audio

You can POST an audio stream to the server for any number of clients to consume it. For example, you can `curl` a local music stream and then POST it:

```
curl http://<someurl>/radio.mp3 | curl -k -H "Transfer-Encoding: chunked" -X POST -T -  'localhost:9222/test.mp3?stream=true'
```

This stream is now accessible at `localhost:9222/test.mp3`. The `?stream=true` flag is important to tell the server to start reading bytes right awawy, even if there is no listener. It has the benefit of immediately sending data to *all listeners* so that you can have multiple connections on that will all receive the data. Another useful flags for streaming is `advertise=true` which will advertise the stream on the main page.

## Installation

First install Go.

```
go install -v github.com/schollz/broadcast-server@latest
```

## License

MIT