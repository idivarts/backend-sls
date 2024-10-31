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

		_, err := svc.CreateJob(createMediaConvertJobInput(s3ObjectKey))

		if err != nil {
			log.Fatalf("Failed to create MediaConvert job: %v", err)
		}
	}
}

func createMediaConvertJobInput(s3ObjectKey string) *mediaconvert.CreateJobInput {
	bucketName := os.Getenv("VIDEO_S3_BUCKET_NAME")
	jobRoleArn := os.Getenv("MEDIACONVERT_ROLE_ARN")
	jobTemplateArn := os.Getenv("MEDIACONVERT_JOBTEMPLATE_ARN")
	jobQueueArn := os.Getenv("MEDIACONVERT_JOBQUEUE_ARN")

	return &mediaconvert.CreateJobInput{
		JobTemplate: aws.String(jobTemplateArn),
		Queue:       aws.String(jobQueueArn),
		Role:        aws.String(jobRoleArn),
		Settings: &mediaconvert.JobSettings{
			OutputGroups: []*mediaconvert.OutputGroup{
				{
					Name: aws.String("File Group"),
					Outputs: []*mediaconvert.Output{
						{
							Preset:       aws.String("System-Generic_Uhd_Mp4_Hevc_Aac_16x9_3840x2160p_24Hz_8Mbps"),
							Extension:    aws.String("mp4"),
							NameModifier: aws.String("_Generic_Uhd_Mp4_Hevc_Aac_16x9_3840x2160p_24Hz_8Mbps"),
						},
						{
							Preset:       aws.String("System-Generic_Hd_Mp4_Hevc_Aac_16x9_1920x1080p_24Hz_4.5Mbps"),
							Extension:    aws.String("mp4"),
							NameModifier: aws.String("_Generic_Hd_Mp4_Hevc_Aac_16x9_1920x1080p_24Hz_4.5Mbps"),
						},
						{
							Preset:       aws.String("System-Generic_Hd_Mp4_Hevc_Aac_16x9_1280x720p_24Hz_3.0Mbps"),
							Extension:    aws.String("mp4"),
							NameModifier: aws.String("_Generic_Hd_Mp4_Hevc_Aac_16x9_1280x720p_24Hz_3.0Mbps"),
						},
						{
							Preset:       aws.String("System-Generic_Hd_Mp4_Avc_Aac_16x9_1920x1080p_24Hz_6Mbps"),
							Extension:    aws.String("mp4"),
							NameModifier: aws.String("_Generic_Hd_Mp4_Avc_Aac_16x9_1920x1080p_24Hz_6Mbps"),
						},
						{
							Preset:       aws.String("System-Generic_Hd_Mp4_Avc_Aac_16x9_1280x720p_24Hz_4.5Mbps"),
							Extension:    aws.String("mp4"),
							NameModifier: aws.String("_Generic_Hd_Mp4_Avc_Aac_16x9_1280x720p_24Hz_4.5Mbps"),
						},
						{
							Preset:       aws.String("System-Generic_Sd_Mp4_Avc_Aac_4x3_640x480p_24Hz_1.5Mbps"),
							Extension:    aws.String("mp4"),
							NameModifier: aws.String("_Generic_Sd_Mp4_Avc_Aac_4x3_640x480p_24Hz_1.5Mbps"),
						},
					},
					OutputGroupSettings: &mediaconvert.OutputGroupSettings{
						Type: aws.String("FILE_GROUP_SETTINGS"),
						FileGroupSettings: &mediaconvert.FileGroupSettings{
							Destination: aws.String(fmt.Sprintf("s3://%s/outputs/%s", bucketName, s3ObjectKey)),
						},
					},
				},
			},
			AdAvailOffset: aws.Int64(0),
			Inputs: []*mediaconvert.Input{
				{
					AudioSelectors: map[string]*mediaconvert.AudioSelector{
						"Audio Selector 1": {
							DefaultSelection: aws.String("DEFAULT"),
						},
					},
					VideoSelector:  &mediaconvert.VideoSelector{},
					TimecodeSource: aws.String("ZEROBASED"),
					FileInput:      aws.String(fmt.Sprintf("s3://%s/%s", bucketName, s3ObjectKey)),
				},
			},
		},
		BillingTagsSource:    aws.String("JOB"),
		AccelerationSettings: &mediaconvert.AccelerationSettings{Mode: aws.String("DISABLED")},
		StatusUpdateInterval: aws.String("SECONDS_60"),
		Priority:             aws.Int64(0),
		HopDestinations:      []*mediaconvert.HopDestination{},
		UserMetadata:         map[string]*string{},
	}
}
