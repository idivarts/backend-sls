package videos

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/mediaconvert"
)

func VideoProcessHandler(ctx context.Context, s3Event events.S3Event) {
	for _, record := range s3Event.Records {
		s3ObjectKey := record.S3.Object.Key
		fmt.Printf("Processing S3 object %s\n", s3ObjectKey)

		sess := session.Must(session.NewSessionWithOptions(session.Options{
			SharedConfigState: session.SharedConfigEnable,
		}))
		svc := mediaconvert.New(sess)

		// Get s3 bucket name from environment variable
		bucketName := os.Getenv("VIDEO_S3_BUCKET_NAME")
		jobRoleArn := os.Getenv("MEDIACONVERT_ROLE_ARN")

		_, err := svc.CreateJob(&mediaconvert.CreateJobInput{
			Role: aws.String(jobRoleArn),
			Settings: &mediaconvert.JobSettings{
				Inputs:       []*mediaconvert.Input{{FileInput: aws.String(fmt.Sprintf("s3://%s/%s", bucketName, s3ObjectKey))}},
				OutputGroups: []*mediaconvert.OutputGroup{
					// Add your output configurations here, e.g., HLS or MP4
				},
			},
		})

		if err != nil {
			log.Fatalf("Failed to create MediaConvert job: %v", err)
		}
	}
}
