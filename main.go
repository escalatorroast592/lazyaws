package main

import (
	"log"
	"context"
	"os/exec"
	"strings"
	"github.com/rivo/tview"
	"github.com/gdamore/tcell/v2"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func getAWSProfiles() ([]string, error) {
	cmd := exec.Command("aws", "configure", "list-profiles")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	return lines, nil
}

func main() {
	app := tview.NewApplication()

	profilePanel := tview.NewList().ShowSecondaryText(false)
	mainPanel := tview.NewTextView().SetText("Select a functionality from the left panel.")

	profiles, err := getAWSProfiles()
	if err != nil || len(profiles) == 0 {
		mainPanel.SetText("Failed to load AWS profiles. Is AWS CLI installed and configured?")
		profiles = []string{"default"}
	}

	selectedProfile := profiles[0]

	// Define flex before using it in the profilePanel callback
	var flex *tview.Flex

	menu := tview.NewList().
		AddItem("S3", "Manage S3 buckets", '1', func() {
			mainPanel.SetText("Loading S3 buckets...")
			go func() {
				cfgOpts := []func(*config.LoadOptions) error{config.WithSharedConfigProfile(selectedProfile)}
				cfg, err := config.LoadDefaultConfig(context.Background(), cfgOpts...)
				if err != nil {
					app.QueueUpdateDraw(func() {
						mainPanel.SetText("Failed to load AWS config: " + err.Error())
					})
					return
				}
				s3Client := s3.NewFromConfig(cfg)
				result, err := s3Client.ListBuckets(context.Background(), &s3.ListBucketsInput{})
				if err != nil {
					app.QueueUpdateDraw(func() {
						mainPanel.SetText("Failed to list S3 buckets: " + err.Error())
					})
					return
				}
				bucketList := tview.NewList().ShowSecondaryText(false)
				for _, b := range result.Buckets {
					name := *b.Name
					bucketList.AddItem(name, "", 0, nil)
				}
				bucketList.SetBorder(true).SetTitle("S3 Buckets (use arrows)")
				bucketList.SetDoneFunc(func() {
					app.SetRoot(flex, true)
				})
				app.QueueUpdateDraw(func() {
					app.SetRoot(bucketList, true)
				})
			}()
		}).
		AddItem("EC2", "Manage EC2 instances", '2', nil).
		AddItem("CodePipeline", "Manage CodePipelines", '3', nil).
		AddItem("Lambda", "Manage Lambda functions", '4', nil).
		AddItem("Quit", "Exit LazyAWS", 'q', func() { app.Stop() })

	menu.SetChangedFunc(func(index int, mainText string, secondaryText string, shortcut rune) {
		if mainText != "S3" {
			mainPanel.SetText("Selected: " + mainText)
		}
	})

	flex = tview.NewFlex().
		AddItem(menu, 30, 1, true).
		AddItem(mainPanel, 0, 3, false)

	for _, p := range profiles {
		p := p
		profilePanel.AddItem(p, "", 0, func() {
			selectedProfile = p
			app.SetRoot(flex, true)
		})
	}

	profilePanel.SetBorder(true).SetTitle("Select AWS Profile")

	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Rune() {
		case 'q', 'Q':
			app.Stop()
		}
		if event.Key() == tcell.KeyCtrlC {
			app.Stop()
		}
		return event
	})

	app.SetRoot(profilePanel, true)
	if err := app.Run(); err != nil {
		log.Fatalf("Error running application: %v", err)
	}
}
