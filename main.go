package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	// Import data layer functions from awsdata.go
)

// Add import for data functions
//go:generate go run awsdata.go

// Add cache variables for AWS resources
var (
	cachedBuckets   map[string][]string = make(map[string][]string) // profile -> buckets
	cachedPipelines map[string][]string = make(map[string][]string) // profile -> pipelines
	cachedLambdas   map[string][]string = make(map[string][]string) // profile -> lambdas
)

func getAWSProfiles() ([]string, error) {
	return ListAWSProfiles()
}

// Extract: Loads S3 buckets and updates the UI with a navigable list
// Add a filter input for bucket names
func showS3Buckets(app *tview.Application, flex *tview.Flex, mainPanel *tview.TextView, menu *tview.List, selectedProfile string, focusedPanel *int, bucketList **tview.List, contentPanel *tview.Primitive) {
	log.Println("Starting goroutine to load S3 buckets")
	
	go func() {
		var buckets []string
		var err error
		if b, ok := cachedBuckets[selectedProfile]; ok {
			buckets = b
		} else {
			buckets, err = ListS3Buckets(selectedProfile)
			if err == nil {
				cachedBuckets[selectedProfile] = buckets
			}
		}
		if err != nil {
			log.Println("Failed to list S3 buckets:", err)
			app.QueueUpdateDraw(func() {
				mainPanel.SetText("Failed to list S3 buckets: " + err.Error())
			})
			return
		}
		log.Println("Fetched buckets, count:", len(buckets))

		// Store all bucket names for filtering
		allBuckets := buckets

		// Create filter input and bucket list
		filterInput := tview.NewInputField().SetLabel("Filter: ")
		filterInput.SetBackgroundColor(tcell.ColorDefault)
		filterInput.SetFieldBackgroundColor(tcell.ColorDefault)
		filterInput.SetBorder(true).SetTitle("Bucket Filter")
		bucketListWidget := tview.NewList().ShowSecondaryText(false)
		
		bucketListWidget.SetBackgroundColor(tcell.ColorDefault)

		// After updateBucketList("") and before filterPanel creation
		bucketCount := len(allBuckets)
		bucketListWidget.SetBorder(true).SetTitle(fmt.Sprintf("S3 Buckets (%d)", bucketCount))

		updateBucketList := func(filter string) {
			bucketListWidget.Clear()
			visible := 0
			selectedIdx := -1
			for i, name := range allBuckets {
				if filter == "" || strings.Contains(strings.ToLower(name), strings.ToLower(filter)) {
					bucketListWidget.AddItem(name, "", 0, func(bucketName string) func() {
						return func() {
							showS3BucketContentsPanel(app, flex, menu, selectedProfile, bucketName, focusedPanel, contentPanel)
						}
					}(name))
					visible++
					if selectedIdx == -1 {
						selectedIdx = i
					}
				}
			}
			current := bucketListWidget.GetCurrentItem()
			if current < 0 {
				current = 0
			}
			// Show selected bucket number (1-based) if any are visible
			selectedDisplay := ""
			if visible > 0 {
				selectedDisplay = fmt.Sprintf(" | Selected: %d", current+1)
			}
			bucketListWidget.SetTitle(fmt.Sprintf("S3 Buckets (%d/%d%s)", visible, len(allBuckets), selectedDisplay))
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

		// Also update the title on selection change
		bucketListWidget.SetChangedFunc(func(index int, mainText, secondaryText string, shortcut rune) {
			visible := bucketListWidget.GetItemCount()
			selectedDisplay := ""
			if visible > 0 {
				selectedDisplay = fmt.Sprintf(" | Selected: %d", index+1)
			}
			bucketListWidget.SetTitle(fmt.Sprintf("S3 Buckets (%d/%d%s)", visible, len(allBuckets), selectedDisplay))
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

// showS3BucketContentsPanel displays the contents of a selected S3 bucket in a new panel.
// It allows the user to download objects and updates the UI with object counts and selection info.
func showS3BucketContentsPanel(app *tview.Application, flex *tview.Flex, menu *tview.List, selectedProfile string, bucketName string, focusedPanel *int, contentPanel *tview.Primitive) {
	objectList := createObjectListPanel(app, flex, menu, selectedProfile, bucketName, focusedPanel, contentPanel)
	// Remove previous content panel if present
	if *contentPanel != nil {
		flex.RemoveItem(*contentPanel)
	}
	*contentPanel = objectList
	flex.AddItem(objectList, 0, 2, false)
	*focusedPanel = 2
	app.SetFocus(objectList)
}

// createObjectListPanel creates and returns a tview.List panel for displaying S3 objects in a bucket.
func createObjectListPanel(app *tview.Application, flex *tview.Flex, menu *tview.List, selectedProfile, bucketName string, focusedPanel *int, contentPanel *tview.Primitive) *tview.List {
	objectList := tview.NewList().ShowSecondaryText(false)
	objectList.SetBorder(true).SetTitle("Contents: " + bucketName)
	objectList.SetBackgroundColor(tcell.ColorDefault)
	objectList.SetMainTextStyle(tcell.StyleDefault)

	// Set up done handler to remove the content panel and return focus to the bucket list
	objectList.SetDoneFunc(func() {
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
				go downloadS3Object(app, selectedProfile, bucketName, mainText)
			}
			return nil // prevent further handling
		}
		return event
	})

	// Load objects in a goroutine and update the UI
	go loadAndDisplayS3Objects(app, objectList, selectedProfile, bucketName)

	return objectList
}

// loadAndDisplayS3Objects fetches S3 objects and updates the object list panel.
func loadAndDisplayS3Objects(app *tview.Application, objectList *tview.List, selectedProfile, bucketName string) {
	log.Println("Loading contents for bucket:", bucketName)
	objects, err := ListS3Objects(selectedProfile, bucketName)
	if err != nil {
		log.Println("Failed to list objects in bucket:", err)
		app.QueueUpdateDraw(func() {
			objectList.AddItem("Failed to list objects in bucket: "+err.Error(), "", 0, nil)
		})
		return
	}
	if len(objects) == 0 {
		app.QueueUpdateDraw(func() {
			objectList.AddItem("(Bucket is empty)", "", 0, nil)
		})
	} else {
		for _, key := range objects {
			objectList.AddItem(key, "", 0, func() {
				log.Println("Selected object:", key)
				// You can add more actions here for the selected object
			})
		}
	}
	objectCount := len(objects)
	objectList.SetTitle(fmt.Sprintf("Contents: %s (%d)", bucketName, objectCount))
	app.QueueUpdateDraw(func() {
		objectList.SetCurrentItem(0)
	})
	objectList.SetChangedFunc(func(index int, mainText, secondaryText string, shortcut rune) {
		visible := objectList.GetItemCount()
		selectedDisplay := ""
		if visible > 0 {
			selectedDisplay = fmt.Sprintf(" | %d", index+1)
		}
		objectList.SetTitle(fmt.Sprintf("Contents: %s (%d%s)", bucketName, visible, selectedDisplay))
	})
}

// Download the selected S3 object to the current directory
func downloadS3Object(app *tview.Application, selectedProfile, bucketName, objectKey string) {
	app.QueueUpdateDraw(func() {
		
	})
	_, err := DownloadS3Object(selectedProfile, bucketName, objectKey)
	if err != nil {
		log.Printf("Failed to download %s: %v", objectKey, err)
		app.QueueUpdateDraw(func() {
			
		})
		return
	}
	log.Printf("Downloaded %s successfully", objectKey)
	app.QueueUpdateDraw(func() {
	
	})
}

// Panel creation utility
func createPanels() (services *tview.List, resources *tview.List, profilePanel *tview.List, mainFlex *tview.Flex) {
	profilePanel = tview.NewList().ShowSecondaryText(false)
	profilePanel.SetBorder(true).SetTitle("Select AWS Profile")

	services = tview.NewList().ShowSecondaryText(false)
	services.SetBorder(true).SetTitle("Services")
	services.ShowSecondaryText(false)

	resources = tview.NewList().ShowSecondaryText(false)
	resources.SetBorder(true).SetTitle("Resources")

	leftFlex := tview.NewFlex().SetDirection(tview.TabSize).
		AddItem(services, 0, 1, true).
		AddItem(resources, 0, 2, false)
	mainFlex = tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(leftFlex, 50, 2, true)

	return services, resources, profilePanel, mainFlex
}

func main() {
	// Log to file
	logFile, err := os.OpenFile("lazyaws.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	log.SetOutput(logFile)

	app := tview.NewApplication()

	menu, resourceList, profilePanel, flex := createPanels()

	profiles, err := getAWSProfiles()

	selectedProfile := profiles[0]

	// Define flex before using it in the profilePanel callback
	// New: Add a resource list panel below the menu

	// Track which panel is focused: 0 = menu, 1 = resourceList, 2 = contentPanel
	focusedPanel := 0
	var bucketList *tview.List // no longer used for left panel, but keep for content
	var contentPanel tview.Primitive

	// Declare menu before its use in resourceList
	menu = tview.NewList().
		AddItem("S3", "", '1', func() {
			// When S3 is selected, show S3 buckets in resourceList
			resourceList.Clear()
			resourceList.SetTitle("S3 Buckets")
			var buckets []string
			var err error
			if b, ok := cachedBuckets[selectedProfile]; ok {
				buckets = b
			} else {
				buckets, err = ListS3Buckets(selectedProfile)
				if err == nil {
					cachedBuckets[selectedProfile] = buckets
				}
			}
			if err != nil {
				resourceList.AddItem("Failed to load buckets: "+err.Error(), "", 0, nil)
				return
			}
			for _, name := range buckets {
				resourceList.AddItem(name, "", 0, func(bucketName string) func() {
					return func() {
						showS3BucketContentsPanel(app, flex, menu, selectedProfile, bucketName, &focusedPanel, &contentPanel)
					}
				}(name))
			}
			if resourceList.GetItemCount() > 0 {
				resourceList.SetCurrentItem(0)
			}
			app.SetFocus(resourceList)
			focusedPanel = 1
		}).
		AddItem("CodePipeline", "Manage CodePipelines", '3', func() {
			resourceList.Clear()
			resourceList.SetTitle("Pipelines")
			var pipelines []string
			var err error
			if p, ok := cachedPipelines[selectedProfile]; ok {
				pipelines = p
			} else {
				pipelines, err = ListCodePipelines(selectedProfile)
				if err == nil {
					cachedPipelines[selectedProfile] = pipelines
				}
			}
			if err != nil {
				resourceList.AddItem("Failed to load pipelines: "+err.Error(), "", 0, nil)
				return
			}
			for _, name := range pipelines {
				resourceList.AddItem(name, "", 0, func(pipelineName string) func() {
					return func() {
						showCodePipelineDetails(app, flex, menu, selectedProfile, pipelineName, &focusedPanel, &contentPanel)
					}
				}(name))
			}
			if resourceList.GetItemCount() > 0 {
				resourceList.SetCurrentItem(0)
			}
			app.SetFocus(resourceList)
			focusedPanel = 1
		}).
		AddItem("Lambda", "Manage Lambda functions", '4', func() {
			resourceList.Clear()
			resourceList.SetTitle("Lambdas")
			var lambdas []string
			var err error
			if l, ok := cachedLambdas[selectedProfile]; ok {
				lambdas = l
			} else {
				lambdas, err = ListLambdas(selectedProfile)
				if err == nil {
					cachedLambdas[selectedProfile] = lambdas
				}
			}
			if err != nil {
				resourceList.AddItem("Failed to load lambdas: "+err.Error(), "", 0, nil)
				return
			}
			for _, name := range lambdas {
				resourceList.AddItem(name, "", 0, nil)
			}
			if resourceList.GetItemCount() > 0 {
				resourceList.SetCurrentItem(0)
			}
			app.SetFocus(resourceList)
			focusedPanel = 1
		}).
		AddItem("Quit", "Exit LazyAWS", 'q', func() { app.Stop() })


	menu.SetBorder(true).SetTitle("Functionalities")
	menu.SetBackgroundColor(tcell.ColorDefault)
	menu.SetMainTextStyle(tcell.StyleDefault)
	menu.SetSecondaryTextStyle(tcell.StyleDefault)
	menu.ShowSecondaryText(false)

	// New flex layout: left column is menu + resourceList, right is mainPanel/content
	leftFlex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(menu, 0, 1, true).
		AddItem(resourceList, 0, 2, false)
	flex = tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(leftFlex, 30, 1, true)

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
			//add log
			
			// Cycle focus through all open panels (menu, mainPanel, bucketList, contentPanel)
			panels := []tview.Primitive{menu}
			if bucketList != nil && flex.GetItemCount() > 1 {
				panels = append(panels, bucketList)
			}
			if contentPanel != nil && flex.GetItemCount() > 2 {
				panels = append(panels, contentPanel)
			}
			// Find current focus and move to next
			current := app.GetFocus()
			//log current 
		
			idx := 0
			for i, p := range panels {
				if current == p {
					idx = i
					break
				}
			}
			next := (idx + 1) % len(panels)
			app.SetFocus(panels[next])
			// Set border color for all panels
			for i, p := range panels {
				setPanelBorderColor(app, p, i == next)
			}
			return nil // prevent further handling
		}
		return event
	})

	// After each panel creation and when focus changes, call setPanelBorderColor to update border color
	// For initial state, set menu as focused
	setPanelBorderColor(app, menu, true)
	if bucketList != nil {
		setPanelBorderColor(app, bucketList, false)
	}
	if contentPanel != nil {
		setPanelBorderColor(app, contentPanel, false)
	}

	app.SetRoot(profilePanel, true)
	if err := app.Run(); err != nil {
		log.Fatalf("Error running application: %v", err)
	}
}

// Show details for a selected CodePipeline
func showCodePipelineDetails(app *tview.Application, flex *tview.Flex, menu *tview.List, selectedProfile, pipelineName string, focusedPanel *int, contentPanel *tview.Primitive) {
	
	go func() {
		details, stageStates, err := GetCodePipelineDetails(selectedProfile, pipelineName)
		if err != nil {
			log.Println("Failed to get pipeline details:", err)
			app.QueueUpdateDraw(func() {
				
			})
			return
		}

		// Format details for display
		detailText := ""
		if details != nil && details.Pipeline != nil {
			p := details.Pipeline
			detailText += fmt.Sprintf("Name: %s\nRole ARN: %s\nVersion: %d\nArtifact Store: %s\n",
				* p.Name, *p.RoleArn, p.Version, *p.ArtifactStore.Location)
			if len(p.Stages) > 0 {
				detailText += "Stages:\n"
				for _, stage := range p.Stages {
					detailText += fmt.Sprintf("  - %s\n", *stage.Name)
				}
			}
		}
		if len(stageStates) > 0 {
			detailText += "\nStage Statuses:\n"
			for stageName, state := range stageStates {
				status := ""
				if len(state.ActionStates) > 0 {
					for _, action := range state.ActionStates {
						if action.LatestExecution != nil {
							status = string(action.LatestExecution.Status)
							break
						}
					}
				}
				detailText += fmt.Sprintf("  %s: %s\n", stageName, status)
			}
		}
		if detailText == "" {
			detailText = "No details found for pipeline."
		}
		app.QueueUpdateDraw(func() {
			
			*focusedPanel = 2
			
			if *contentPanel != nil {
				flex.RemoveItem(*contentPanel)
			}
		})
	}()
}
