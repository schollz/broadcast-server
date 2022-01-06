# broadcast

A simple Go server that broadcasts any data/stream

## usage

### streaming

You can POST a stream to the server. For example, you can `curl` a local stream and then POST it:

```
curl http://<someurl>/radio.mp3 | curl -k -H "Transfer-Encoding: chunked" -X POST -T -  localhost:9222/test.mp3
```

This stream is now accessible at `localhost:9222/test.mp3`.

### data

You could also POST data. 

```
curl -X POST --data-binary "@111.png" localhost:9222/test.png
```

In this case, the data is posted to all current listeners immediately. That means that the clients receiving the data *must be connected before the data is posted*.