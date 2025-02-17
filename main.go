package main

import (
	"github.com/bushubdegefu/blue-proxy.com/manager"
)

//	@title			Swagger  API
//	@version		0.1
//	@description	This is  API OPENAPI Documentation.
//	@termsOfService	http://swagger.io/terms/
//  @BasePath  /api/v1

//	@securityDefinitions.apikey	ApiKeyAuth
//	@in							header
//	@name						X-APP-TOKEN
//	@description				Description for what is this security definition being used

//	@securityDefinitions.apikey Refresh
//	@in							header
//	@name						X-REFRESH-TOKEN
//	@description				Description for what is this security definition being used

func main() {
	manager.Execute()
}
