@echo off
setlocal enabledelayedexpansion

@REM run build to build
@REM run docker to run
@REM run kill to kill

for %%a in (%*) do set "%%~a=1"

if "%build%"=="1" SET "GOARCH=amd64" && SET "GOOS=linux" && SET "CGO_ENABLED=0" && echo Building Go project... && go build -o ./bin/kademlia_linux.exe ./cmd/main.go && echo Docker stuff... && docker build . -t kadlab || exit /b 1
if "%kill%"=="1" docker swarm leave --force
if "%docker%"=="1" docker swarm init && docker stack deploy --detach=true -c docker-compose.yml nodestack || exit /b 1