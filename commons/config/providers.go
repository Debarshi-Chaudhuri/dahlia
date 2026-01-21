package config

import (
	"context"
	"dahlia/commons/routes"
	cache "dahlia/internal/cache/iface"
	redisCache "dahlia/internal/cache/redis"
	coordinator "dahlia/internal/coordinator/iface"
	zkCoordinator "dahlia/internal/coordinator/zk"
	"dahlia/internal/logger"
	"dahlia/internal/slack"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awsdynamodb "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/gin-gonic/gin"
	"go.uber.org/fx/fxevent"
)

// ProvideLogger creates and configures the logger for the application
func ProvideLogger() (logger.Logger, error) {
	return logger.NewZapLoggerForDev()
}

// ProvideFxLogger creates the FX event logger using the application logger
func ProvideFxLogger(log logger.Logger) fxevent.Logger {
	return &fxevent.ZapLogger{
		Logger: log.(*logger.ZapLogger).Logger(),
	}
}

// ProvideRouteDependencies creates route dependencies
func ProvideRouteDependencies(log logger.Logger) routes.RouteDependencies {
	return routes.RouteDependencies{
		Logger: log,
	}
}

// ProvideRouter creates and configures the Gin router with all routes
func ProvideRouter(
	config routes.RouterConfig,
	deps routes.RouteDependencies,
	routeInitializer func(*gin.Engine, routes.RouteDependencies),
) *gin.Engine {
	router := routes.NewRouter(config, deps)
	routeInitializer(router, deps)
	return router
}

// ProvideSQSClient provides an SQS client (for LocalStack or AWS)
func initializeSqsClient(endpoint, region string) (*sqs.Client, error) {
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(region),
		config.WithEndpointResolverWithOptions(aws.EndpointResolverWithOptionsFunc(
			func(service, region string, options ...interface{}) (aws.Endpoint, error) {
				if endpoint != "" {
					return aws.Endpoint{
						URL:           endpoint,
						SigningRegion: region,
					}, nil
				}
				return aws.Endpoint{}, &aws.EndpointNotFoundError{}
			})),
	)
	if err != nil {
		return nil, err
	}

	return sqs.NewFromConfig(cfg), nil
}

func ProvideSQSClient() (*sqs.Client, error) {
	return initializeSqsClient("http://localhost:4566", "us-east-1")
}

// ProvideSlackClient provides a Slack client (mock for development)
func ProvideSlackClient(log logger.Logger) slack.Client {
	return slack.NewMockClient(log)
}

// ProvideZooKeeperCoordinator provides a ZooKeeper coordinator for distributed coordination
func ProvideZooKeeperCoordinator(log logger.Logger) (coordinator.Coordinator, error) {
	servers := []string{"localhost:2181"}
	sessionTimeout := 60 * time.Second // Increased for debugging and stability

	coord, err := zkCoordinator.NewZKCoordinator(servers, sessionTimeout, log)
	if err != nil {
		return nil, err
	}

	return coord, nil
}

// ProvideRedisCache provides a Redis cache client
func ProvideRedisCache(log logger.Logger) (cache.Cache, error) {
	addr := "localhost:6379"
	password := "" // No password for local development
	db := 0        // Default DB

	cache, err := redisCache.NewRedisCache(addr, password, db, log)
	if err != nil {
		return nil, err
	}

	return cache, nil
}

// ProvideDynamoDBClient provides DynamoDB client
func ProvideDynamoDBClient() (*awsdynamodb.Client, error) {
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion("us-east-1"),
		awsconfig.WithEndpointResolverWithOptions(aws.EndpointResolverWithOptionsFunc(
			func(service, region string, options ...interface{}) (aws.Endpoint, error) {
				return aws.Endpoint{
					URL:           "http://localhost:9000",
					SigningRegion: region,
				}, nil
			})),
	)
	if err != nil {
		return nil, err
	}

	return awsdynamodb.NewFromConfig(cfg), nil
}
