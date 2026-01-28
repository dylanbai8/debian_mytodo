package main

import (
	"encoding/json"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"time"
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

const (
	iconFile = "tray.png"
	dataFile = "todo.json"
	appID    = "io.github.dylan.todo.tray"
	maxLen   = 50 // 每条最多50汉字
)

type Todo struct {
	Text string `json:"text"`
}

func loadTodos() ([]Todo, error) {
	if _, err := os.Stat(dataFile); os.IsNotExist(err) {
		return []Todo{}, nil
	}
	data, err := os.ReadFile(dataFile)
	if err != nil {
		return nil, err
	}
	var todos []Todo
	return todos, json.Unmarshal(data, &todos)
}

func saveTodos(todos []Todo) {
	data, _ := json.MarshalIndent(todos, "", "  ")
	_ = os.WriteFile(dataFile, data, 0644)
}

// ensureIconFile 生成极简待办事项图标（透明背景+黑色线条）
func ensureIconFile() string {
	if _, err := os.Stat(iconFile); err == nil {
		abs, _ := filepath.Abs(iconFile)
		return abs
	}

	// 创建32x32透明背景图像
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{0, 0, 0, 0}}, image.Point{}, draw.Src)
	black := color.RGBA{0, 0, 0, 255}

	// 绘制极简清单图标：3个复选框 + 对应横线（左侧对齐，简洁布局）
	// 复选框位置：(6,8), (6,14), (6,20) —— 每个复选框3x3像素
	// 横线位置：从x=12开始，长度15像素，y对应复选框中间位置
	checkSize := 3 // 复选框边长（像素）
	lineStartX := 12
	lineLength := 15
	lineYOffsets := []int{9, 15, 21} // 横线垂直位置（对应复选框中间）

	for i := 0; i < 3; i++ {
		x := 6
		y := 8 + i*6 // 每个复选框垂直间隔6像素

		// 绘制复选框（空心正方形，1像素边框）
		// 上边
		for dx := 0; dx < checkSize; dx++ {
			img.Set(x+dx, y, black)
		}
		// 下边
		for dx := 0; dx < checkSize; dx++ {
			img.Set(x+dx, y+checkSize-1, black)
		}
		// 左边
		for dy := 0; dy < checkSize; dy++ {
			img.Set(x, y+dy, black)
		}
		// 右边
		for dy := 0; dy < checkSize; dy++ {
			img.Set(x+checkSize-1, y+dy, black)
		}

		// 绘制右侧横线（1像素高度）
		lineY := lineYOffsets[i]
		for dx := 0; dx < lineLength; dx++ {
			img.Set(lineStartX+dx, lineY, black)
		}
	}

	// 保存为PNG文件
	f, err := os.Create(iconFile)
	if err != nil {
		log.Fatal("create icon failed:", err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		log.Fatal("encode png failed:", err)
	}

	abs, _ := filepath.Abs(iconFile)
	return abs
}

func showTemporaryPopUp(c fyne.Canvas, text string, seconds float64) {
	label := widget.NewLabel(text)
	label.Alignment = fyne.TextAlignCenter

	pop := widget.NewPopUp(container.NewCenter(label), c)
	pop.Show()

	go func() {
		time.Sleep(time.Duration(seconds * float64(time.Second)))
		fyne.Do(func() {
			pop.Hide()
		})
	}()
}

func main() {
	a := app.NewWithID(appID)
	iconPath := ensureIconFile()

	todos, err := loadTodos()
	if err != nil {
		log.Fatal(err)
	}

	listBox := container.NewVBox()
	input := widget.NewEntry()
	input.SetPlaceHolder("新增待办事项，回车确认（最多50字）")

	win := a.NewWindow("待办事项")
	win.Resize(fyne.NewSize(360, 440))
	win.SetFixedSize(false)
	win.SetCloseIntercept(func() {
		win.Hide()
	})

	var refreshList func()
	refreshList = func() {
		listBox.Objects = nil
		for i, todo := range todos {
			index := i

			label := widget.NewLabel(todo.Text)
			label.Wrapping = fyne.TextWrapWord
			label.Alignment = fyne.TextAlignLeading

			copyBtn := widget.NewButton("复制", func() {
				a.Clipboard().SetContent(todo.Text)
				showTemporaryPopUp(win.Canvas(), "已复制到剪贴板", 2)
			})
			copyBtn.Importance = widget.LowImportance

			check := widget.NewCheck("", func(done bool) {
				if done {
					todos = append(todos[:index], todos[index+1:]...)
					saveTodos(todos)
					refreshList()
				}
			})

			// 核心布局：左侧复选框 + 中间文字（自动填充） + 右侧复制按钮
			row := container.NewBorder(nil, nil, check, copyBtn, label)
			card := container.NewVBox(row, widget.NewSeparator())
			listBox.Add(card)
		}
		listBox.Refresh()
	}

	// 输入框回车事件（限制长度）
	input.OnSubmitted = func(text string) {
		if text == "" {
			return
		}
		if utf8.RuneCountInString(text) > maxLen {
			showTemporaryPopUp(win.Canvas(), "待办事项最多50个汉字", 2)
			return
		}
		todos = append(todos, Todo{Text: text})
		saveTodos(todos)
		input.SetText("")
		refreshList()
	}

	// 窗口布局：底部输入框 + 滚动列表
	win.SetContent(container.NewBorder(
		nil,
		container.NewVBox(widget.NewSeparator(), input),
		nil,
		nil,
		container.NewVScroll(container.NewBorder(nil, nil, nil, layout.NewSpacer(), listBox)),
	))

	refreshList()
	win.Hide()

	// 系统托盘设置
	if tray, ok := a.(desktop.App); ok {
		res, err := fyne.LoadResourceFromPath(iconPath)
		if err != nil {
			log.Fatal("load tray icon failed:", err)
		}
		tray.SetSystemTrayIcon(res)

		tray.SetSystemTrayMenu(fyne.NewMenu("Todo",
			fyne.NewMenuItem("打开待办事项", func() {
				fyne.Do(func() {
					win.Show()
					win.RequestFocus()
				})
			}),
			fyne.NewMenuItem("退出", func() {
				a.Quit()
			}),
		))
	}

	a.Run()
}
