package main

import (
	"context"
	"os/exec"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/codepipeline"
	"github.com/aws/aws-sdk-go-v2/service/codepipeline/types"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// Data layer: All AWS API/CLI calls and data transformation

func ListAWSProfiles() ([]string, error) {
	cmd := exec.Command("aws", "configure", "list-profiles")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	return lines, nil
}

func ListS3Buckets(profile string) ([]string, error) {
	cfgOpts := []func(*config.LoadOptions) error{config.WithSharedConfigProfile(profile)}
	cfg, err := config.LoadDefaultConfig(context.Background(), cfgOpts...)
	if err != nil {
		return nil, err
	}
	s3Client := s3.NewFromConfig(cfg)
	result, err := s3Client.ListBuckets(context.Background(), &s3.ListBucketsInput{})
	if err != nil {
		return nil, err
	}
	buckets := make([]string, 0, len(result.Buckets))
	for _, b := range result.Buckets {
		buckets = append(buckets, *b.Name)
	}
	return buckets, nil
}

func ListS3Objects(profile, bucket string) ([]string, error) {
	cfgOpts := []func(*config.LoadOptions) error{config.WithSharedConfigProfile(profile)}
	cfg, err := config.LoadDefaultConfig(context.Background(), cfgOpts...)
	if err != nil {
		return nil, err
	}
	s3Client := s3.NewFromConfig(cfg)
	result, err := s3Client.ListObjectsV2(context.Background(), &s3.ListObjectsV2Input{
		Bucket: &bucket,
	})
	if err != nil {
		return nil, err
	}
	objects := make([]string, 0, len(result.Contents))
	for _, obj := range result.Contents {
		objects = append(objects, *obj.Key)
	}
	return objects, nil
}

func DownloadS3Object(profile, bucket, key string) (string, error) {
	cmd := exec.Command("aws", "s3", "cp", "s3://"+bucket+"/"+key, "./"+key, "--profile", profile)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func ListCodePipelines(profile string) ([]string, error) {
	cfgOpts := []func(*config.LoadOptions) error{config.WithSharedConfigProfile(profile)}
	cfg, err := config.LoadDefaultConfig(context.Background(), cfgOpts...)
	if err != nil {
		return nil, err
	}
	client := codepipeline.NewFromConfig(cfg)
	result, err := client.ListPipelines(context.Background(), &codepipeline.ListPipelinesInput{})
	if err != nil {
		return nil, err
	}
	pipelines := make([]string, 0, len(result.Pipelines))
	for _, p := range result.Pipelines {
		pipelines = append(pipelines, *p.Name)
	}
	return pipelines, nil
}

func GetCodePipelineDetails(profile, pipelineName string) (*codepipeline.GetPipelineOutput, map[string]types.StageState, error) {
	cfgOpts := []func(*config.LoadOptions) error{config.WithSharedConfigProfile(profile)}
	cfg, err := config.LoadDefaultConfig(context.Background(), cfgOpts...)
	if err != nil {
		return nil, nil, err
	}
	client := codepipeline.NewFromConfig(cfg)
	pipe, err := client.GetPipeline(context.Background(), &codepipeline.GetPipelineInput{
		Name: &pipelineName,
	})
	if err != nil {
		return nil, nil, err
	}
	stateResp, err := client.GetPipelineState(context.Background(), &codepipeline.GetPipelineStateInput{
		Name: &pipelineName,
	})
	stageStates := make(map[string]types.StageState)
	if err == nil {
		for _, s := range stateResp.StageStates {
			if s.StageName != nil {
				stageStates[*s.StageName] = s
			}
		}
	}
	return pipe, stageStates, nil
}

func GetCodePipelineActionLogs(profile, pipelineName string) (string, error) {
	cmd := exec.Command("aws", "codepipeline", "get-pipeline-state", "--name", pipelineName, "--profile", profile)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func ListLambdas(profile string) ([]string, error) {
	cfgOpts := []func(*config.LoadOptions) error{config.WithSharedConfigProfile(profile)}
	cfg, err := config.LoadDefaultConfig(context.Background(), cfgOpts...)
	if err != nil {
		return nil, err
	}
	client := lambda.NewFromConfig(cfg)
	result, err := client.ListFunctions(context.Background(), &lambda.ListFunctionsInput{})
	if err != nil {
		return nil, err
	}
	lambdas := make([]string, 0, len(result.Functions))
	for _, f := range result.Functions {
		lambdas = append(lambdas, *f.FunctionName)
	}
	return lambdas, nil
}
