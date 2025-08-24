package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	docStyle          = lipgloss.NewStyle().Margin(1, 2)
	helpStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("#626262"))
	errorStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("#d70000"))
	successStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#00d787")).Bold(true)
	headerStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5faf")).Bold(true)
	activeItemStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#d75fd7")).Bold(true)
	inactiveItemStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff")).PaddingLeft(2)
	codeStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("#5fd7ff")).Background(lipgloss.Color("#303030")).Padding(0, 1)
)

const FabricGitRepositoryUrl = "https://github.com/Fabric-Development/fabric"
const GithubApiExamplesUrl = "https://api.github.com/repos/Fabric-Development/fabric/contents/examples"

type githubContent struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	DownloadURL string `json:"download_url"`
}

type programState int

const (
	statePromptVenv programState = iota
	stateCreateVenv
	stateInstallFabric
	statePromptStubs
	stateGenerateStubs
	stateListExamples
	statePromptExample
	stateFetchExample
	stateDone
)

// example files list
type item string

func (i item) FilterValue() string { return string(i) }

type itemDelegate struct{}

func (d itemDelegate) Height() int                               { return 1 }
func (d itemDelegate) Spacing() int                              { return 0 }
func (d itemDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd { return nil }
func (d itemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(item)
	if !ok {
		return
	}

	itemStr := fmt.Sprintf("%d. %s", index+1, i)
	if index == m.Index() { // item is active
		itemStr = activeItemStyle.Render("> " + itemStr)
	} else {
		itemStr = inactiveItemStyle.Render(itemStr)
	}

	fmt.Fprint(w, itemStr)
}

type model struct {
	state         programState
	venvNameInput textinput.Model
	stubsInput    textinput.Model
	exampleList   list.Model
	spinner       spinner.Model
	progress      progress.Model
	progressMsg   string
	venvPath      string
	sitePackages  string
	finalMessage  string
	width, height int
	err           error
}

type venvCreatedMsg struct{ venvPath string }
type fabricInstalledMsg struct{ sitePackages string }
type stubsGeneratedMsg struct{}
type examplesListedMsg struct{ examples []string }
type exampleFetchedMsg struct{ name string }
type errMsg struct{ err error }

func runCommandSilent(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("command '%s %s' failed: %v\n%s", name, strings.Join(args, " "), err, stderr.String())
	}
	return nil
}

func doCreateVenv(venvName string) tea.Msg {
	if err := runCommandSilent("python", "-m", "venv", venvName); err != nil {
		return errMsg{err}
	}
	absPath, err := filepath.Abs(venvName)
	if err != nil {
		return errMsg{err}
	}
	return venvCreatedMsg{absPath}
}

func doInstallFabric(venvPath string) tea.Msg {
	pipPath := filepath.Join(venvPath, "bin", "pip")
	if err := runCommandSilent(pipPath, "install", "git+"+FabricGitRepositoryUrl, "psutil"); err != nil {
		return errMsg{err}
	}

	pythonPath := filepath.Join(venvPath, "bin", "python")
	out, err := exec.Command(pythonPath, "-c", "import site; print(site.getsitepackages()[0])").Output()
	if err != nil {
		return errMsg{fmt.Errorf("could not determine site-packages path: %w", err)}
	}
	return fabricInstalledMsg{strings.TrimSpace(string(out))}
}

func doGenerateStubs(modules []string, sitePackages string) tea.Msg {
	GenerateStubs(modules, filepath.Join(sitePackages, "gi-stubs"), false)
	return stubsGeneratedMsg{}
}

func doListExamples() tea.Msg {
	resp, err := http.Get(GithubApiExamplesUrl)
	if err != nil {
		return errMsg{err}
	}
	defer resp.Body.Close()

	var contents []githubContent
	if err := json.NewDecoder(resp.Body).Decode(&contents); err != nil {
		return errMsg{err}
	}

	var examples []string
	for _, item := range contents {
		if item.Type == "dir" {
			examples = append(examples, item.Name)
		}
	}
	return examplesListedMsg{examples}
}

func downloadFile(url, path string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}

func fetchExampleFile(name string) tea.Msg {
	if name == "" || name == "None (skip)" {
		return exampleFetchedMsg{"none"}
	}

	// conflict checking
	checkURL := fmt.Sprintf("%s/%s", GithubApiExamplesUrl, name)
	resp, err := http.Get(checkURL)
	if err != nil {
		return errMsg{err}
	}
	defer resp.Body.Close()

	var contents []githubContent
	if err := json.NewDecoder(resp.Body).Decode(&contents); err != nil {
		return errMsg{err}
	}

	for _, item := range contents {
		if item.Type != "file" {
			continue
		}

		if _, err := os.Stat(item.Name); err == nil {
			return errMsg{fmt.Errorf("file '%s' already exists in the current directory, aborting", item.Name)}
		}
	}

	// all aboard...
	for _, item := range contents {
		if item.Type != "file" {
			continue
		}

		if err := downloadFile(item.DownloadURL, item.Name); err != nil {
			return errMsg{err}
		}
	}
	return exampleFetchedMsg{name}
}

func initialModel() model {
	tiVenv := textinput.New()
	tiVenv.Placeholder = "venv"
	tiVenv.Focus()
	tiVenv.CharLimit = -1
	tiVenv.Width = 30

	tiStubs := textinput.New()
	tiStubs.Placeholder = "Gtk-3.0, Playerctl-2.0"
	tiStubs.CharLimit = -1
	tiStubs.Width = 50

	exampleList := list.New([]list.Item{}, itemDelegate{}, 0, 0)
	exampleList.Title = "Which example would you like to start with?"
	exampleList.SetShowHelp(false)

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	p := progress.New(progress.WithDefaultGradient())

	return model{
		state:         statePromptVenv,
		venvNameInput: tiVenv,
		stubsInput:    tiStubs,
		exampleList:   exampleList,
		spinner:       s,
		progress:      p,
	}
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit
		}

		switch m.state { // if something is spinning don't interact
		case stateCreateVenv, stateInstallFabric, stateGenerateStubs, stateListExamples, stateFetchExample:
			return m, nil
		}

		switch msg.Type {
		case tea.KeyEnter:
			switch m.state {
			case statePromptVenv:
				m.state = stateCreateVenv
				venvName := m.venvNameInput.Value()
				if venvName == "" {
					venvName = "venv"
				}
				m.progressMsg = fmt.Sprintf("Creating Python virtual environment in './%s'...", venvName)
				return m, tea.Batch(m.spinner.Tick, func() tea.Msg { return doCreateVenv(venvName) })
			case statePromptStubs:
				m.state = stateGenerateStubs
				stubsValue := m.stubsInput.Value()
				if stubsValue == "" {
					stubsValue = m.stubsInput.Placeholder
				}
				m.progressMsg = fmt.Sprintf("Generating stubs for '%s'...", stubsValue)
				modules := strings.Split(strings.ReplaceAll(stubsValue, " ", ""), ",")
				return m, tea.Batch(m.spinner.Tick, func() tea.Msg { return doGenerateStubs(modules, m.sitePackages) })
			case statePromptExample:
				var ok bool
				var selectedItem item
				if selectedItem, ok = m.exampleList.SelectedItem().(item); !ok {
					break
				}

				m.state = stateFetchExample
				exampleName := string(selectedItem)
				if exampleName == "None (skip)" {
					m.progressMsg = "Skipping example download..."
				} else {
					m.progressMsg = fmt.Sprintf("Fetching example '%s'...", exampleName)
				}
				return m, tea.Batch(m.spinner.Tick, func() tea.Msg { return fetchExampleFile(exampleName) })
			}
		}

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.exampleList.SetWidth(msg.Width)
		m.exampleList.SetHeight(msg.Height - 4)

	case venvCreatedMsg:
		m.state = stateInstallFabric
		m.venvPath = msg.venvPath
		m.progressMsg = "Virtual environment created. Installing Fabric..."
		return m, func() tea.Msg { return doInstallFabric(m.venvPath) }

	case fabricInstalledMsg:
		m.stubsInput.Focus()
		m.state = statePromptStubs
		m.sitePackages = msg.sitePackages
		return m, textinput.Blink

	case stubsGeneratedMsg:
		m.state = stateListExamples
		m.progressMsg = "Stubs generated. Fetching available example configureations..."
		return m, tea.Batch(m.spinner.Tick, func() tea.Msg { return doListExamples() })

	case examplesListedMsg:
		m.state = statePromptExample
		items := []list.Item{item("None (skip)")}
		for _, ex := range msg.examples {
			items = append(items, item(ex))
		}
		m.exampleList.SetItems(items)
		return m, nil

	case exampleFetchedMsg:
		m.state = stateDone
		var builder strings.Builder
		builder.WriteString(successStyle.Render("✓ Fabric environment is ready!") + "\n\n")
		builder.WriteString(fmt.Sprintf("A Python virtual environment has been created at: %s\n", m.venvPath))
		builder.WriteString("Fabric and type stubs for your libraries have been installed inside it.\n\n")
		builder.WriteString(headerStyle.Render("IMPORTANT: You must activate the environment in your shell.") + "\n")
		builder.WriteString("Run the following command to begin:\n\n")
		builder.WriteString(codeStyle.Render(fmt.Sprintf("source %s/bin/activate", filepath.Base(m.venvPath))) + "\n\n")

		if msg.name != "none" {
			builder.WriteString(fmt.Sprintf("The '%s' example files have been downloaded to the current directory.\n", msg.name))
		}
		m.finalMessage = builder.String()
		return m, tea.Quit

	case errMsg:
		m.err = msg.err
		return m, tea.Quit

	case spinner.TickMsg:
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case progress.FrameMsg:
		progressModel, cmd := m.progress.Update(msg)
		m.progress = progressModel.(progress.Model)
		return m, cmd
	}

	// inputs
	switch m.state {
	case statePromptVenv:
		m.venvNameInput, cmd = m.venvNameInput.Update(msg)
	case statePromptStubs:
		m.stubsInput, cmd = m.stubsInput.Update(msg)
	case statePromptExample:
		m.exampleList, cmd = m.exampleList.Update(msg)
	}

	return m, cmd
}

func (m model) View() string {
	if m.err != nil {
		return docStyle.Render(fmt.Sprintf("\nAn error occurred:\n\n%s\n\nPress any key to exit.", errorStyle.Render(m.err.Error())))
	}
	if m.finalMessage != "" {
		return docStyle.Render(m.finalMessage)
	}

	var s string
	switch m.state {
	case statePromptVenv:
		s = lipgloss.JoinVertical(lipgloss.Left,
			headerStyle.Render("Fabric Environment Setup"),
			"\nEnter a name for the new Python virtual environment:",
			m.venvNameInput.View(),
			helpStyle.Render("\n(default: venv) (press Enter to confirm)"),
		)
	case stateCreateVenv, stateInstallFabric, stateGenerateStubs, stateListExamples, stateFetchExample:
		s = fmt.Sprintf("\n%s %s\n", m.spinner.View(), m.progressMsg)
	case statePromptStubs:
		s = lipgloss.JoinVertical(lipgloss.Left,
			successStyle.Render("✓ Virtual environment created and Fabric installed!"),
			headerStyle.Render("\nWhat libraries would you like to generate stubs for?"),
			m.stubsInput.View(),
			helpStyle.Render("\n(comma-separated, e.g., Gtk-3.0, Gray-0.1) (default: Gtk-3.0, Playerctl-2.0)"),
		)
	case statePromptExample:
		s = lipgloss.JoinVertical(lipgloss.Left,
			successStyle.Render("✓ Type stubs generated successfully!"),
			"\n"+m.exampleList.View(),
		)
	}
	return docStyle.Render(s)
}

func InitInteractive() {
	if _, err := exec.LookPath("python"); err != nil {
		fmt.Println(docStyle.Render(errorStyle.Render("ERROR: python not found.") + "\nPlease install Python to continue."))
		os.Exit(1)
	}

	p := tea.NewProgram(initialModel())
	if _, err := p.Run(); err != nil {
		fmt.Printf("An error occurred during initialization: %v\n", err)
		os.Exit(1)
	}
}
