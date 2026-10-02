package apihandler

import (
	"context"
	"log"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	ginadapter "github.com/awslabs/aws-lambda-go-api-proxy/gin"
	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/pkg/middlewares"
	"github.com/idivarts/backend-sls/pkg/mysentry"
	"github.com/idivarts/backend-sls/pkg/myutil"
)

var ginLambda *ginadapter.GinLambda
var GinEngine *gin.Engine

func init() {
	// stdout and stderr are sent to AWS CloudWatch Logs
	log.Printf("Gin cold start")
	GinEngine = gin.Default()
	// GinEngine.Use(cors.New(cors.Config{
	// 	AllowOrigins: []string{"*"}, // You can specify the allowed origins here
	// 	AllowMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
	// 	AllowHeaders: []string{"Origin", "Content-Type"},
	// }))
	GinEngine.Use(middlewares.CORSMiddleware())

	// Every Gin-based lambda shares this engine, so wiring Sentry here covers
	// all of them at once. No-op unless SENTRY_DSN is set.
	mysentry.Init()
	if mw := mysentry.GinMiddleware(); mw != nil {
		GinEngine.Use(mw)
	}
}

func Handler(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	// Lambda freezes this environment the moment we return, which would strand
	// anything Sentry has buffered. Flush while we still have execution time.
	defer mysentry.Flush()

	// If no name is provided in the HTTP request body, throw an error
	return ginLambda.ProxyWithContext(ctx, req)
}
func StartLambda() {
	ginLambda = ginadapter.New(GinEngine)
	if myutil.IsDevEnvironment() {
		ginLambda.StripBasePath("/dev")
	}
	lambda.Start(Handler)
}
