# audio.exploreapollo.org

Audio Processing Server for exploreapollo.org

Exposes an audio stream to be consumed by the client.

## Parameters

| Parameter | Domain | Description |
|-----------|------|-------------|
| `mission` | INT | Apollo mission of interest |
| `channels` | []INT | Channels of interest (comma separated) |
| `format` | `m4a`, `ogg` | streaming format |
| `start` | INT64 | Mission Elapsed Time, in milliseconds |
| `duration` | INT64 | duration of desired audio beginning at time `start`, in milliseconds |

## Example Query Url

http://audio.exploreapollo.org/stream?mission=11&channels=14,18,24&start=369300000&duration=600000&format=m4a

returns an m4a stream of Apollo 11 channels 14, 18, and 24 starting at MET 369300000 (this is equivalent to MET 102:35:00) and lasting 600000 milliseconds (10 minutes).
