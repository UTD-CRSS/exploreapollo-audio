# audio.exploreapollo.org

Audio Processing Server for exploreapollo.org

Exposes an audio stream to be consumed by the client.


## Dev Setup

1. Install Go:  
  sudo add-apt-repository ppa:ubuntu-lxc/lxd-stable  
  sudo apt-get update  
  sudo apt-get install golang  

2. Create Go workspace & clone repository  
  mkdir $HOME/exploreapollo-audio  
  export GOPATH=$HOME/work       (this can be anywhere, maybe add this to ~/.bashrc)  
  mkdir $GOPATH/src/github.com/UTD-CRSS  
  cd $GOPATH/src/github.com/UTD-CRSS  
  git clone https://github.com/UTD-CRSS/exploreapollo-audio  
  mv exploreapollo-audio audio.exploreapollo.org  
  cd audio.exploreapollo.org  
  cp sample-config.json config.json  
3. Install psql driver && compile project  
  go get github.com/lib/pq  
  go install github.com/UTD-CRSS/exploreapollo-audio/audio  	
  
  to run server just: go run main.go
  

## Parameters

| Parameter | Domain | Description |
|-----------|------|-------------|
| `mission` | INT | Apollo mission of interest |
| `channels` | []INT | Channels of interest (comma separated) |
| `format` | `m4a`, `ogg` | streaming format |
| `start` | INT64 | Mission Elapsed Time, in milliseconds |
| `duration` | INT64 | duration of desired audio beginning at time `start`, in milliseconds |

## Example Query Url

`http://audio.exploreapollo.org/stream?mission=11&channels=14,18,24&start=369300000&duration=600000&format=m4a`

returns an m4a stream of Apollo 11 channels 14, 18, and 24 starting at MET 369300000 (this is equivalent to MET 102:35:00) and lasting 600000 milliseconds (10 minutes).

## Heroku deployment

The server uses the following buildpacks, which must be installed
via the Heroku console for deployment:

- https://github.com/heroku/heroku-buildpack-apt
- https://github.com/lespreludes/heroku-buildpack-ffmpeg-lame.git
- https://github.com/heroku/heroku-buildpack-go.git

The file Aptfile is used by heroku-buildpack-apt to obtain additional dependencies.
