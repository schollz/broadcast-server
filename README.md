# broadcast

A simple Go server that broadcasts any data/stream

## usage

### data

You can POST data. 

```
curl -X POST --data-binary "@111.png" localhost:9222/test.png
```

You can even post movies and such which will stream to multiple people:


### streaming

You can POST a stream to the server. For example, you can `curl` a local stream and then POST it:

```
curl http://<someurl>/radio.mp3 | curl -k -H "Transfer-Encoding: chunked" -X POST -T -  'localhost:9222/test.mp3?stream=true'
```

This stream is now accessible at `localhost:9222/test.mp3`. The `?stream=true` flag is important to tell the server to start reading bytes right awawy, even if there is no listener. it has the benefit of immediately sending data to *all listeners* so that you can have multiple connections on that will all receive the data.
