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
		Settings: &mediaconvert.JobSettings{
			OutputGroups: []*mediaconvert.OutputGroup{
				{
					Name: aws.String("CMAF"),
					Outputs: []*mediaconvert.Output{
						{
							Preset:       aws.String("System-Ott_Cmaf_Cmfc_Avc_16x9_Sdr_1920x1080p_30Hz_10Mbps_Cbr"),
							NameModifier: aws.String("_Ott_Cmaf_Cmfc_Avc_16x9_Sdr_1920x1080p_30Hz_10000Kbps_Cbr"),
						},
						{
							Preset:       aws.String("System-Ott_Cmaf_Cmfc_Avc_16x9_Sdr_1920x1080p_30Hz_8Mbps_Cbr"),
							NameModifier: aws.String("_Ott_Cmaf_Cmfc_Avc_16x9_Sdr_1920x1080p_30Hz_8000Kbps_Cbr"),
						},
						{
							Preset:       aws.String("System-Ott_Cmaf_Cmfc_Avc_16x9_Sdr_1440x810p_30Hz_6Mbps_Cbr"),
							NameModifier: aws.String("_Ott_Cmaf_Cmfc_Avc_16x9_Sdr_1440x810p_30Hz_6000Kbps_Cbr"),
						},
						{
							Preset:       aws.String("System-Ott_Cmaf_Cmfc_Avc_16x9_Sdr_1440x810p_30Hz_5Mbps_Cbr"),
							NameModifier: aws.String("_Ott_Cmaf_Cmfc_Avc_16x9_Sdr_1440x810p_30Hz_5000Kbps_Cbr"),
						},
						{
							Preset:       aws.String("System-Ott_Cmaf_Cmfc_Avc_16x9_Sdr_1280x720p_30Hz_5Mbps_Cbr"),
							NameModifier: aws.String("_Ott_Cmaf_Cmfc_Avc_16x9_Sdr_1280x720p_30Hz_5000Kbps_Cbr"),
						},
						{
							Preset:       aws.String("System-Ott_Cmaf_Cmfc_Avc_16x9_Sdr_1280x720p_30Hz_4Mbps_Cbr"),
							NameModifier: aws.String("_Ott_Cmaf_Cmfc_Avc_16x9_Sdr_1280x720p_30Hz_4000Kbps_Cbr"),
						},
						{
							Preset:       aws.String("System-Ott_Cmaf_Cmfc_Avc_16x9_Sdr_960x540p_30Hz_2.5Mbps_Cbr"),
							NameModifier: aws.String("_Ott_Cmaf_Cmfc_Avc_16x9_Sdr_960x540p_30Hz_2500Kbps_Cbr"),
						},
						{
							Preset:       aws.String("System-Ott_Cmaf_Cmfc_Avc_16x9_Sdr_768x432p_30Hz_1.2Mbps_Cbr"),
							NameModifier: aws.String("_Ott_Cmaf_Cmfc_Avc_16x9_Sdr_768x432p_30Hz_1200Kbps_Cbr"),
						},
						{
							Preset:       aws.String("System-Ott_Cmaf_Cmfc_Avc_16x9_Sdr_640x360p_30Hz_0.8Mbps_Cbr"),
							NameModifier: aws.String("_Ott_Cmaf_Cmfc_Avc_16x9_Sdr_640x360p_30Hz_800Kbps_Cbr"),
						},
						{
							Preset:       aws.String("System-Ott_Cmaf_Cmfc_Avc_16x9_Sdr_416x234p_30Hz_0.36Mbps_Cbr"),
							NameModifier: aws.String("_Ott_Cmaf_Cmfc_Avc_16x9_Sdr_416x234p_30Hz_360Kbps_Cbr"),
						},
						{
							Preset:       aws.String("System-Ott_Cmaf_Cmfc_Aac_He_96Kbps"),
							NameModifier: aws.String("_Ott_Cmaf_Cmfc_Aac_He_96Kbps"),
						},
						{
							Preset:       aws.String("System-Ott_Cmaf_Cmfc_Aac_He_64Kbps"),
							NameModifier: aws.String("_Ott_Cmaf_Cmfc_Aac_He_64Kbps"),
						},
					},
					OutputGroupSettings: &mediaconvert.OutputGroupSettings{
						Type: aws.String("CMAF_GROUP_SETTINGS"),
						CmafGroupSettings: &mediaconvert.CmafGroupSettings{
							WriteHlsManifest:       aws.String("ENABLED"),
							WriteDashManifest:      aws.String("ENABLED"),
							SegmentLength:          aws.Int64(30),
							FragmentLength:         aws.Int64(3),
							SegmentControl:         aws.String("SEGMENTED_FILES"),
							ManifestDurationFormat: aws.String("INTEGER"),
							StreamInfResolution:    aws.String("INCLUDE"),
							ClientCache:            aws.String("ENABLED"),
							ManifestCompression:    aws.String("NONE"),
							CodecSpecification:     aws.String("RFC_4281"),
							Destination:            aws.String(fmt.Sprintf("s3://%s/outputs/", bucketName)),
						},
					},
				},
			},
			AdAvailOffset: aws.Int64(0),
			Inputs: []*mediaconvert.Input{
				{
					TimecodeSource: aws.String("ZEROBASED"),
					VideoSelector:  &mediaconvert.VideoSelector{},
					AudioSelectors: map[string]*mediaconvert.AudioSelector{
						"Audio Selector 1": {
							DefaultSelection: aws.String("DEFAULT"),
						},
					},
					FileInput: aws.String(fmt.Sprintf("s3://%s/%s", bucketName, s3ObjectKey)),
				},
			},
			FollowSource: aws.Int64(1),
		},
		AccelerationSettings: &mediaconvert.AccelerationSettings{
			Mode: aws.String("DISABLED"),
		},
		HopDestinations: []*mediaconvert.HopDestination{},
		JobTemplate:     aws.String(jobTemplateArn),
		Queue:           aws.String(jobQueueArn),
		Role:            aws.String(jobRoleArn),
	}
}
