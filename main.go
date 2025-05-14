package main

import (
	"context"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/codepipeline"
	"github.com/aws/aws-sdk-go-v2/service/codepipeline/types"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
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

// Extract: Loads S3 buckets and updates the UI with a navigable list
// Add a filter input for bucket names
func showS3Buckets(app *tview.Application, flex *tview.Flex, mainPanel *tview.TextView, menu *tview.List, selectedProfile string, focusedPanel *int, bucketList **tview.List, contentPanel *tview.Primitive) {
	mainPanel.SetText("Loading S3 buckets...")
	log.Println("Starting goroutine to load S3 buckets")
	
	go func() {
		cfgOpts := []func(*config.LoadOptions) error{config.WithSharedConfigProfile(selectedProfile)}
		cfg, err := config.LoadDefaultConfig(context.Background(), cfgOpts...)
		if err != nil {
			log.Println("Failed to load AWS config:", err)
			app.QueueUpdateDraw(func() {
				mainPanel.SetText("Failed to load AWS config: " + err.Error())
			})
			return
		}
		log.Println("Loaded AWS config, creating S3 client")
		s3Client := s3.NewFromConfig(cfg)
		result, err := s3Client.ListBuckets(context.Background(), &s3.ListBucketsInput{})
		if err != nil {
			log.Println("Failed to list S3 buckets:", err)
			app.QueueUpdateDraw(func() {
				mainPanel.SetText("Failed to list S3 buckets: " + err.Error())
			})
			return
		}
		log.Println("Fetched buckets, count:", len(result.Buckets))

		// Store all bucket names for filtering
		allBuckets := make([]string, 0, len(result.Buckets))
		for _, b := range result.Buckets {
			allBuckets = append(allBuckets, *b.Name)
		}

		// Create filter input and bucket list
		filterInput := tview.NewInputField().SetLabel("Filter: ")
		filterInput.SetBackgroundColor(tcell.ColorDefault)
		filterInput.SetFieldBackgroundColor(tcell.ColorDefault)
		filterInput.SetBorder(true).SetTitle("Bucket Filter")
		bucketListWidget := tview.NewList().ShowSecondaryText(false)
		updateBucketList := func(filter string) {
			bucketListWidget.Clear()
			for _, name := range allBuckets {
				if filter == "" || strings.Contains(strings.ToLower(name), strings.ToLower(filter)) {
					bucketListWidget.AddItem(name, "", 0, func(bucketName string) func() {
						return func() {
							showS3BucketContentsPanel(app, flex, mainPanel, menu, selectedProfile, bucketName, focusedPanel, contentPanel)
						}
					}(name))
				}
			}
			if bucketListWidget.GetItemCount() > 0 {
				bucketListWidget.SetCurrentItem(0)
			}
		}
		updateBucketList("")

		// Create a vertical flex for the bucket list and a filter panel at the bottom
		filterPanel := tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(bucketListWidget, 0, 1, true).
			AddItem(filterInput, 3, 0, false)
		filterPanel.SetBorder(false)

		bucketListWidget.SetBorder(true).SetTitle("S3 Buckets (use arrows)")
		bucketListWidget.SetBackgroundColor(tcell.ColorDefault)
		bucketListWidget.SetMainTextStyle(tcell.StyleDefault)
		bucketListWidget.SetDoneFunc(func() {
			log.Println("bucketList done, restoring mainPanel")
			app.QueueUpdateDraw(func() {
				flex.RemoveItem(filterPanel)
				flex.AddItem(mainPanel, 0, 3, false)
				*focusedPanel = 0
				app.SetFocus(menu)
				*bucketList = nil // allow reopening
				log.Println("bucketList set to nil after done")
			})
		})

		filterInput.SetChangedFunc(func(text string) {
			updateBucketList(text)
		})
		filterInput.SetDoneFunc(func(key tcell.Key) {
			if key == tcell.KeyUp || key == tcell.KeyEnter {
				app.SetFocus(bucketListWidget)
			}
		})

		// Add shortcut to focus filter input when 'f' is pressed in the bucket list
		bucketListWidget.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
			if event.Rune() == 'f' || event.Rune() == 'F' {
				app.SetFocus(filterInput)
				return nil
			}
			return event
		})

		app.QueueUpdateDraw(func() {
			log.Println("Switching to bucketList panel with filter at bottom")
			flex.RemoveItem(mainPanel)
			flex.AddItem(filterPanel, 0, 3, false)
			*focusedPanel = 1
			app.SetFocus(bucketListWidget)
			*bucketList = bucketListWidget
		})
	}()
}

func showS3BucketContentsPanel(app *tview.Application, flex *tview.Flex, mainPanel *tview.TextView, menu *tview.List, selectedProfile string, bucketName string, focusedPanel *int, contentPanel *tview.Primitive) {
	objectList := tview.NewList().ShowSecondaryText(false)
	objectList.SetBorder(true).SetTitle("Contents: " + bucketName)
	objectList.SetDoneFunc(func() {
		// Remove the content panel when done
		app.QueueUpdateDraw(func() {
			flex.RemoveItem(objectList)
			*contentPanel = nil
			*focusedPanel = 1
			if flex.GetItemCount() > 1 {
				app.SetFocus(flex.GetItem(1)) // focus bucket list
			}
		})
	})

	// Add shortcut for downloading the selected file
	objectList.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Rune() == 'd' || event.Rune() == 'D' {
			index := objectList.GetCurrentItem()
			mainText, _ := objectList.GetItemText(index)
			if mainText != "(Bucket is empty)" && !strings.HasPrefix(mainText, "Failed") {
				go downloadS3Object(app, selectedProfile, bucketName, mainText, mainPanel)
			}
			return nil // prevent further handling
		}
		return event
	})

	log.Println("Loading contents for bucket:", bucketName)
	go func() {
		cfgOpts := []func(*config.LoadOptions) error{config.WithSharedConfigProfile(selectedProfile)}
		cfg, err := config.LoadDefaultConfig(context.Background(), cfgOpts...)
		if err != nil {
			log.Println("Failed to load AWS config for bucket content:", err)
			app.QueueUpdateDraw(func() {
				objectList.AddItem("Failed to load AWS config: "+err.Error(), "", 0, nil)
			})
			return
		}
		s3Client := s3.NewFromConfig(cfg)
		result, err := s3Client.ListObjectsV2(context.Background(), &s3.ListObjectsV2Input{
			Bucket: &bucketName,
		})
		if err != nil {
			log.Println("Failed to list objects in bucket:", err)
			app.QueueUpdateDraw(func() {
				objectList.AddItem("Failed to list objects in bucket: "+err.Error(), "", 0, nil)
			})
			return
		}
		if len(result.Contents) == 0 {
			app.QueueUpdateDraw(func() {
				objectList.AddItem("(Bucket is empty)", "", 0, nil)
			})
		} else {
			for _, obj := range result.Contents {
				key := *obj.Key
				objectList.AddItem(key, "", 0, func() {
					log.Println("Selected object:", key)
					// You can add more actions here for the selected object
				})
			}
		}
		app.QueueUpdateDraw(func() {
			objectList.SetCurrentItem(0)
		})
	}()
	// Remove previous content panel if present
	if *contentPanel != nil {
		flex.RemoveItem(*contentPanel)
	}
	*contentPanel = objectList
	flex.AddItem(objectList, 0, 2, false)
	*focusedPanel = 2
	app.SetFocus(objectList)
}

// Download the selected S3 object to the current directory
func downloadS3Object(app *tview.Application, selectedProfile, bucketName, objectKey string, mainPanel *tview.TextView) {
	app.QueueUpdateDraw(func() {
		mainPanel.SetText("Downloading: " + objectKey)
	})
	cmd := exec.Command("aws", "s3", "cp", "s3://"+bucketName+"/"+objectKey, "./"+objectKey, "--profile", selectedProfile)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Failed to download %s: %v", objectKey, err)
		app.QueueUpdateDraw(func() {
			mainPanel.SetText("Failed to download: " + objectKey + "\n" + string(output))
		})
		return
	}
	log.Printf("Downloaded %s successfully", objectKey)
	app.QueueUpdateDraw(func() {
		mainPanel.SetText("Downloaded: " + objectKey)
	})
}

// Show AWS CodePipelines in a navigable list
func showCodePipelines(app *tview.Application, flex *tview.Flex, mainPanel *tview.TextView, menu *tview.List, selectedProfile string, focusedPanel *int, contentPanel *tview.Primitive) {
	mainPanel.SetText("Loading CodePipelines...")
	go func() {
		cfgOpts := []func(*config.LoadOptions) error{config.WithSharedConfigProfile(selectedProfile)}
		cfg, err := config.LoadDefaultConfig(context.Background(), cfgOpts...)
		if err != nil {
			app.QueueUpdateDraw(func() {
				mainPanel.SetText("Failed to load AWS config: " + err.Error())
			})
			return
		}
		client := codepipeline.NewFromConfig(cfg)
		result, err := client.ListPipelines(context.Background(), &codepipeline.ListPipelinesInput{})
		if err != nil {
			app.QueueUpdateDraw(func() {
				mainPanel.SetText("Failed to list CodePipelines: " + err.Error())
			})
			return
		}
		pipelineList := tview.NewList().ShowSecondaryText(false)
		for _, p := range result.Pipelines {
			name := *p.Name
			pipelineList.AddItem(name, "", 0, func(pipelineName string) func() {
				return func() {
					showCodePipelineDetails(app, flex, mainPanel, menu, selectedProfile, pipelineName, focusedPanel, contentPanel)
				}
			}(name))
		}
		pipelineList.SetBorder(true).SetTitle("CodePipelines (use arrows)")
		pipelineList.SetDoneFunc(func() {
			app.QueueUpdateDraw(func() {
				flex.RemoveItem(pipelineList)
				flex.AddItem(mainPanel, 0, 3, false)
				*focusedPanel = 0
				app.SetFocus(menu)
				if contentPanel != nil {
					*contentPanel = nil
				}
			})
		})
		app.QueueUpdateDraw(func() {
			flex.RemoveItem(mainPanel)
			flex.AddItem(pipelineList, 0, 3, false)
			*focusedPanel = 1
			app.SetFocus(pipelineList)
			if contentPanel != nil {
				*contentPanel = pipelineList
			}
		})
	}()
}

// Show details for a selected CodePipeline, with status for each stage and action, in a new navigable panel
func showCodePipelineDetails(app *tview.Application, flex *tview.Flex, mainPanel *tview.TextView, menu *tview.List, selectedProfile, pipelineName string, focusedPanel *int, contentPanel *tview.Primitive) {
	mainPanel.SetText("Loading details for pipeline: " + pipelineName)
	go func() {
		cfgOpts := []func(*config.LoadOptions) error{config.WithSharedConfigProfile(selectedProfile)}
		cfg, err := config.LoadDefaultConfig(context.Background(), cfgOpts...)
		if err != nil {
			app.QueueUpdateDraw(func() {
				mainPanel.SetText("Failed to load AWS config: " + err.Error())
			})
			return
		}
		client := codepipeline.NewFromConfig(cfg)
		// Get pipeline structure
		pipe, err := client.GetPipeline(context.Background(), &codepipeline.GetPipelineInput{
			Name: &pipelineName,
		})
		if err != nil {
			app.QueueUpdateDraw(func() {
				mainPanel.SetText("Failed to get pipeline details: " + err.Error())
			})
			return
		}
		// Get pipeline execution status
		execs, err := client.ListPipelineExecutions(context.Background(), &codepipeline.ListPipelineExecutionsInput{
			PipelineName: &pipelineName,
			MaxResults: awsInt32(1),
		})
		var latestExec *types.PipelineExecutionSummary
		if err == nil && len(execs.PipelineExecutionSummaries) > 0 {
			latestExec = &execs.PipelineExecutionSummaries[0]
		}
		stageStates := make(map[string]types.StageState)
		if latestExec != nil && latestExec.PipelineExecutionId != nil {
			stateResp, err := client.GetPipelineState(context.Background(), &codepipeline.GetPipelineStateInput{
				Name: &pipelineName,
			})
			if err == nil {
				for _, s := range stateResp.StageStates {
					if s.StageName != nil {
						stageStates[*s.StageName] = s
					}
				}
			}
		}
		// Build a navigable list for stages and actions with status
		panel := tview.NewList().ShowSecondaryText(true)
		for _, stage := range pipe.Pipeline.Stages {
			stageName := *stage.Name
			status := "?"
			if s, ok := stageStates[stageName]; ok && s.LatestExecution != nil && s.LatestExecution.Status != "" {
				status = string(s.LatestExecution.Status)
			}
			panel.AddItem("Stage: "+stageName, "Status: "+status, 0, nil)
			// Add actions for this stage
			actionStates := make(map[string]string)
			if s, ok := stageStates[stageName]; ok {
				for _, a := range s.ActionStates {
					if a.ActionName != nil && a.LatestExecution != nil {
						actionStates[*a.ActionName] = string(a.LatestExecution.Status)
					}
				}
			}
			for _, action := range stage.Actions {
				actionName := *action.Name
				actionStatus := actionStates[actionName]
				panel.AddItem("  Action: "+actionName+" ("+string(action.ActionTypeId.Category)+")", "Status: "+actionStatus, 0, func(stageName, actionName string) func() {
					return func() {
						showCodePipelineActionLogs(app, flex, mainPanel, menu, selectedProfile, pipelineName, stageName, actionName, focusedPanel, contentPanel)
					}
				}(stageName, actionName))
			}
		}
		panel.SetBorder(true).SetTitle("Pipeline Details: " + pipelineName)
		panel.SetDoneFunc(func() {
			app.QueueUpdateDraw(func() {
				flex.RemoveItem(panel)
				*contentPanel = nil
				*focusedPanel = 1
				if flex.GetItemCount() > 1 {
					app.SetFocus(flex.GetItem(1)) // focus pipeline list
				}
			})
		})
		app.QueueUpdateDraw(func() {
			if *contentPanel != nil {
				flex.RemoveItem(*contentPanel)
			}
			*contentPanel = panel
			flex.AddItem(panel, 0, 2, false)
			*focusedPanel = 2
			app.SetFocus(panel)
		})
	}()
}

// Show logs for a selected CodePipeline action in a new panel
func showCodePipelineActionLogs(app *tview.Application, flex *tview.Flex, mainPanel *tview.TextView, menu *tview.List, selectedProfile, pipelineName, stageName, actionName string, focusedPanel *int, contentPanel *tview.Primitive) {
	mainPanel.SetText("Loading logs for action: " + actionName)
	go func() {
		// For simplicity, use AWS CLI to get logs (CloudWatch logs integration is required for full logs)
		// This will just show a placeholder or error if not available
		cmd := exec.Command("aws", "codepipeline", "get-pipeline-state", "--name", pipelineName, "--profile", selectedProfile)
		output, err := cmd.CombinedOutput()
		var logText string
		if err != nil {
			logText = "Failed to get logs: " + err.Error() + "\n" + string(output)
		} else {
			logText = string(output)
		}
		panel := tview.NewTextView().SetText(logText)
		panel.SetBorder(true).SetTitle("Logs: " + actionName)
		app.QueueUpdateDraw(func() {
			if *contentPanel != nil {
				flex.RemoveItem(*contentPanel)
			}
			*contentPanel = panel
			flex.AddItem(panel, 0, 2, false)
			*focusedPanel = 2
			app.SetFocus(panel)
		})
	}()
}

// Show AWS Lambda functions in a navigable list
func showLambdas(app *tview.Application, flex *tview.Flex, mainPanel *tview.TextView, menu *tview.List, selectedProfile string, focusedPanel *int, contentPanel *tview.Primitive) {
	mainPanel.SetText("Loading Lambda functions...")
	go func() {
		cfgOpts := []func(*config.LoadOptions) error{config.WithSharedConfigProfile(selectedProfile)}
		cfg, err := config.LoadDefaultConfig(context.Background(), cfgOpts...)
		if err != nil {
			app.QueueUpdateDraw(func() {
				mainPanel.SetText("Failed to load AWS config: " + err.Error())
			})
			return
		}
		client := lambda.NewFromConfig(cfg)
		result, err := client.ListFunctions(context.Background(), &lambda.ListFunctionsInput{})
		if err != nil {
			app.QueueUpdateDraw(func() {
				mainPanel.SetText("Failed to list Lambda functions: " + err.Error())
			})
			return
		}
		lambdaList := tview.NewList().ShowSecondaryText(false)
		for _, f := range result.Functions {
			name := *f.FunctionName
			lambdaList.AddItem(name, "", 0, nil)
		}
		lambdaList.SetBorder(true).SetTitle("Lambda Functions (use arrows)")
		lambdaList.SetDoneFunc(func() {
			app.QueueUpdateDraw(func() {
				flex.RemoveItem(lambdaList)
				flex.AddItem(mainPanel, 0, 3, false)
				*focusedPanel = 0
				app.SetFocus(menu)
				if contentPanel != nil {
					*contentPanel = nil
				}
			})
		})
		app.QueueUpdateDraw(func() {
			flex.RemoveItem(mainPanel)
			flex.AddItem(lambdaList, 0, 3, false)
			*focusedPanel = 1
			app.SetFocus(lambdaList)
			if contentPanel != nil {
				*contentPanel = lambdaList
			}
		})
	}()
}

// Utility: Remove any right-side panels (bucket list, codepipeline, lambda, content, etc.)
func removeRightPanels(flex *tview.Flex, mainPanel *tview.TextView) {
	for flex.GetItemCount() > 1 {
		flex.RemoveItem(flex.GetItem(1))
	}
	flex.AddItem(mainPanel, 0, 3, false)
}

// Helper for *int32
func awsInt32(v int) *int32 {
	t := int32(v)
	return &t
}

// Set transparent background for all tview primitives and remove borders for text panels
func setTransparentBackground(p tview.Primitive) {
	switch v := p.(type) {
	case *tview.TextView:
		v.SetBackgroundColor(tcell.ColorDefault)
		v.SetBorder(false)
	case *tview.List:
		v.SetBackgroundColor(tcell.ColorDefault)
	case *tview.InputField:
		v.SetBackgroundColor(tcell.ColorDefault)
	case *tview.Flex:
		for i := 0; i < v.GetItemCount(); i++ {
			setTransparentBackground(v.GetItem(i))
		}
	}
}

func main() {
	// Log to file
	logFile, err := os.OpenFile("lazyaws.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	log.SetOutput(logFile)

	app := tview.NewApplication()

	profilePanel := tview.NewList().ShowSecondaryText(false)
	mainPanel := tview.NewTextView().SetText("Select a functionality from the left panel.")
	mainPanel.SetBackgroundColor(tcell.ColorDefault)

	profiles, err := getAWSProfiles()
	if err != nil || len(profiles) == 0 {
		mainPanel.SetText("Failed to load AWS profiles. Is AWS CLI installed and configured?")
		profiles = []string{"default"}
	}

	selectedProfile := profiles[0]

	// Define flex before using it in the profilePanel callback
	var flex *tview.Flex

	// Track which panel is focused: 0 = menu, 1 = mainPanel/bucketList
	focusedPanel := 0
	var bucketList *tview.List
	// Declare contentPanel before showS3Buckets so it is in scope
	var contentPanel tview.Primitive

	// Declare menu before its use in bucketList.SetDoneFunc
	var menu *tview.List
	menu = tview.NewList().
		AddItem("S3", "", '1', func() {
			removeRightPanels(flex, mainPanel)
			log.Println("S3 menu item selected. bucketList pointer:", bucketList)
			// Defensive: check if bucketList is not nil and is still in the flex layout
			if bucketList != nil {
				found := false
				for i := 0; i < flex.GetItemCount(); i++ {
					if flex.GetItem(i) == bucketList {
						found = true
						break
					}
				}
				log.Println("bucketList found in flex:", found)
				if found {
					// bucketList is in flex, so just focus it
					app.SetFocus(bucketList)
				} else {
					
				}
			} else {
				showS3Buckets(app, flex, mainPanel, menu, selectedProfile, &focusedPanel, &bucketList, &contentPanel)		
			}
			
		}).
		AddItem("CodePipeline", "Manage CodePipelines", '3', func() {
			removeRightPanels(flex, mainPanel)
			showCodePipelines(app, flex, mainPanel, menu, selectedProfile, &focusedPanel, &contentPanel)
		}).
		AddItem("Lambda", "Manage Lambda functions", '4', func() {
			removeRightPanels(flex, mainPanel)
			showLambdas(app, flex, mainPanel, menu, selectedProfile, &focusedPanel, &contentPanel)
		}).
		AddItem("Quit", "Exit LazyAWS", 'q', func() { app.Stop() })
	menu.SetBorder(true).SetTitle("Functionalities")
	menu.SetBackgroundColor(tcell.ColorDefault)
	menu.SetMainTextStyle(tcell.StyleDefault)
	menu.SetSecondaryTextStyle(tcell.StyleDefault)
	menu.ShowSecondaryText(false)

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

	// In main, after creating all panels and before app.SetRoot, set transparent backgrounds and remove text borders
	setTransparentBackground(menu)
	setTransparentBackground(mainPanel)
	setTransparentBackground(profilePanel)
	setTransparentBackground(flex)

	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Rune() {
		case 'q', 'Q':
			app.Stop()
		}
		if event.Key() == tcell.KeyCtrlC {
			app.Stop()
		}
		if event.Key() == tcell.KeyTAB {
			if focusedPanel == 0 {
				// Focus main panel (bucketList if present, else mainPanel)
				if bucketList != nil && flex.GetItemCount() > 1 {
					// tview.Flex does not have GetItemAt, so we track if bucketList is present by checking if mainPanel is not present
					app.SetFocus(bucketList)
					focusedPanel = 1
				} else {
					app.SetFocus(mainPanel)
					focusedPanel = 1
				}
			} else {
				app.SetFocus(menu)
				focusedPanel = 0
			}
			return nil // prevent further handling
		}
		return event
	})

	app.SetRoot(profilePanel, true)
	if err := app.Run(); err != nil {
		log.Fatalf("Error running application: %v", err)
	}
}
