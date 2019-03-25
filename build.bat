SET CGO_ENABLED=0
SET GOOS=linux
SET GOARCH=amd64

 go build -o ./bin/cloud/electriciot2   apps/electriciot2/main.go