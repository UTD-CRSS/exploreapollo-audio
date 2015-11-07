# audio.exploreapollo.org

Audio Processing Server for exploreapollo.org

Exposes an audio stream to be consumed by the client.

## Parameters

| Parameter | Domain | Description |
|-----------|------|-------------|
| `mission` | INT | Apollo mission of interest |
| `channel` | []INT | Channels of interest |
| `format` | `m4a`, `ogg` | streaming format |
| `t` | INT64 | Mission Elapsed Time, in milliseconds |
| `len` | INT64 | duration of desired audio beginning at time `t`, in milliseconds |

## Example Query URL

`http://audio.exploreapollo.org/stream?mission=11&channel=14&channel=18&channel=24&format=m4a&t=369300000&len=600000`

returns an m4a stream of Apollo 11 channels 14, 18, and 24 starting at MET 369300000 (this is equivalent to MET 102:35:00) and lasting 600000 milliseconds (10 minutes).
