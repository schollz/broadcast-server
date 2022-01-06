# broadcast

A simple Go server that broadcasts any data/stream

## usage

You can POST a stream/data to the server. For example, you can `curl` a local stream and then POST it:

```
curl http://<someurl>/radio.mp3 | curl -k -H "Transfer-Encoding: chunked" -X POST -T -  localhost:9222/test.mp3
```

This stream is now accessible at `localhost:9222/test.mp3`.

