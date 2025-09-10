@echo off
setlocal

@REM run build to build
@REM run docker to run
@REM run kill to kill

if "%1"=="build" SET "GOARCH=amd64" && SET "GOOS=linux" && SET "CGO_ENABLED=0" && echo Building Go project... && go build -o ./bin/kademlia_linux.exe ./cmd/main.go && echo Docker stuff... && docker build . -t kadlab
if "%1"=="docker" docker swarm init && docker stack deploy --detach=true -c docker-compose.yml nodestack
if "%1"=="kill" docker swarm leave --force