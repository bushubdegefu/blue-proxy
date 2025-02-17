package helper

import (
	"fmt"
	"os"
	"text/template"
)

func TargetsFrame() {
	// ####################################################
	//  rabbit template
	targets_tmpl, err := template.New("RenderData").Parse(targetsTemplate)
	if err != nil {
		panic(err)
	}

	targets_file, err := os.Create("targets.json")
	if err != nil {
		panic(err)
	}
	defer targets_file.Close()

	err = targets_tmpl.Execute(targets_file, nil)
	if err != nil {
		panic(err)
	}
}

func EnviromentFrame() {
	// ####################################################
	//  rabbit template
	enviroment_tmpl, err := template.New("RenderData").Parse(envTemplate)
	if err != nil {
		panic(err)
	}

	// creating config folder for .env files
	err = os.MkdirAll("configs", os.ModePerm)
	if err != nil {
		fmt.Printf("Frame - 10: %v\n", err)
	}

	enviroment_file, err := os.Create("configs/.dev.env")
	if err != nil {
		panic(err)
	}

	defer enviroment_file.Close()

	err = enviroment_tmpl.Execute(enviroment_file, nil)
	if err != nil {
		panic(err)
	}
}
func NormalEnviromentFrame() {
	// ####################################################
	//  rabbit template
	enviroment_tmpl, err := template.New("RenderData").Parse(normalTemplate)
	if err != nil {
		panic(err)
	}

	// creating config folder for .env files
	err = os.MkdirAll("configs", os.ModePerm)
	if err != nil {
		fmt.Printf("Frame - 10: %v\n", err)
	}

	enviroment_file, err := os.Create("configs/.env")
	if err != nil {
		panic(err)
	}

	defer enviroment_file.Close()

	err = enviroment_tmpl.Execute(enviroment_file, nil)
	if err != nil {
		panic(err)
	}
}

var targetsTemplate = `
{
  "targets": [
    "https://localhost:8700",
    "https://localhost:8701",
    "https://localhost:8702",
    "https://localhost:8703"
  ]
}
`

var envTemplate = `
APP_NAME=dev
HTTP_PORT=7500
TEST_NAME="Development Development"
BODY_LIMIT=70
READ_BUFFER_SIZE=40
RATE_LIMIT_PER_SECOND=5000

#Observability settings
TRACE_EXPORTER=jaeger
TRACER_HOST=localhost
TRACER_PORT=14317

TARGET_HOST_NAME=somedomain.com
`
var normalTemplate = `
APP_ENV=dev
`
