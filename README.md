# audio.exploreapollo.org

Audio Processing Server for exploreapollo.org

Exposes an audio stream to be consumed by the client.

## Parameters

| Parameter | Expected | Description |
|-----------|------|-------------|
| `mission` | integer | Apollo mission of interest |
| `channel` | INT | Channels of interest |
| `format` | `m4a` or `ogg` | streaming format |
| `t` | BIGINT | Mission Elapsed Time, in milliseconds |
| `len` | INT | duration of desired audio beginning at time `t`, in seconds |

## Example Query URL

`.../stream?mission=11&channel=14&channel=18&channel=24&format=m4a&t=369300000&len=600`

returns an m4a stream of Apollo 11 channels 14, 18, and 24 starting at MET 369300000 (this is equivalent to MET 102:35:00) and lasting 600 seconds (10 minutes).
