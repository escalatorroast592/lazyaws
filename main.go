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
	"os"
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
		blist := tview.NewList().ShowSecondaryText(false)
		for _, b := range result.Buckets {
			name := *b.Name
			blist.AddItem(name, "", 0, func(bucketName string) func() {
				return func() {
					showS3BucketContentsPanel(app, flex, mainPanel, menu, selectedProfile, bucketName, focusedPanel, contentPanel)
				}
			}(name))
		}
		blist.SetBorder(true).SetTitle("S3 Buckets (use arrows)")
		blist.SetDoneFunc(func() {
			log.Println("bucketList done, restoring mainPanel")
			app.QueueUpdateDraw(func() {
				flex.RemoveItem(blist)
				flex.AddItem(mainPanel, 0, 3, false)
				*focusedPanel = 0
				app.SetFocus(menu)
				*bucketList = nil // allow reopening
				log.Println("bucketList set to nil after done")
			})
		})
		app.QueueUpdateDraw(func() {
			log.Println("Switching to bucketList panel")
			flex.RemoveItem(mainPanel)
			flex.AddItem(blist, 0, 3, false)
			*focusedPanel = 1
			app.SetFocus(blist)
			*bucketList = blist
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
		AddItem("S3", "Manage S3 buckets", '1', func() {
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
		AddItem("EC2", "Manage EC2 instances", '2', nil).
		AddItem("CodePipeline", "Manage CodePipelines", '3', nil).
		AddItem("Lambda", "Manage Lambda functions", '4', nil).
		AddItem("Quit", "Exit LazyAWS", 'q', func() { app.Stop() })
	menu.SetBorder(true).SetTitle("Functionalities")

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
