set mypath=%~dp0..\
cd /d %mypath%

cmd /c "del /Q .build"
cmd /c "mkdir .build"

cd /d ./cmd

SET CGO_ENABLED=0
SET GOOS=darwin
SET GOARCH=amd64
go build -o ../.build/discovery-syncer-%GOOS%-%GOARCH%

SET CGO_ENABLED=0
SET GOOS=windows
SET GOARCH=amd64
go build -o ../.build/discovery-syncer-%GOOS%-%GOARCH%.exe

SET CGO_ENABLED=0
SET GOOS=linux
SET GOARCH=amd64
go build -o ../.build/discovery-syncer-%GOOS%-%GOARCH%


cd /d %mypath%
