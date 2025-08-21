package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/godbus/dbus/v5"
	"github.com/urfave/cli/v2"
)

const (
	FABRIC_DBUS_INTERFACE_NAME = "org.Fabric.fabric"
	FABRIC_DBUS_OBJECT_PATH    = "/org/Fabric/fabric"
)

func handleError(errorMessage string, json bool) {
	if json {
		errorMessage = serializeData(map[string]string{"error": errorMessage})
	}
	fmt.Println(errorMessage)
	os.Exit(1)
}

func getArg(ctx *cli.Context, argIndex int, argName string, errorJson bool) string {
	// urfave's cli library doesn't support named arguments...
	// or does it?
	rawArg := ctx.Args().Get(argIndex)
	if rawArg == "" {
		handleError(fmt.Sprintf("missing argument: %s", argName), errorJson)
	}
	return rawArg
}

func serializeData(data any) string {
	jsonData, err := json.MarshalIndent(data, "", "\t")
	if err != nil {
		fmt.Println(err)
		return ""
	}
	return string(jsonData)
}

func getDBusNames() ([]string, error) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return nil, err
	}

	obj := conn.Object("org.freedesktop.DBus", "/org/freedesktop/DBus")

	var names []string
	if err := obj.Call("org.freedesktop.DBus.ListNames", 0).Store(&names); err != nil {
		return nil, err
	}
	return names, nil
}

func isNameRunning(name string) (bool, error) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return false, err
	}

	obj := conn.Object("org.freedesktop.DBus", "/org/freedesktop/DBus")

	var hasOwner bool
	if err = obj.Call("org.freedesktop.DBus.NameHasOwner", 0, name).Store(&hasOwner); err != nil {
		return false, err
	}
	return hasOwner, nil
}

func getInstanceProxy(ifaceName string) (dbus.BusObject, error) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return nil, err
	}

	obj := conn.Object(ifaceName, dbus.ObjectPath(FABRIC_DBUS_OBJECT_PATH))
	return obj, nil
}

func checkAndGetInstanceProxy(configName string, errorJson bool) (dbus.BusObject, error) {
	var ifaceName string
	if strings.HasPrefix(configName, FABRIC_DBUS_INTERFACE_NAME) {
		ifaceName = configName
	} else {
		ifaceName = FABRIC_DBUS_INTERFACE_NAME
		if configName != "" {
			ifaceName += "." + configName
		}
	}

	running, err := isNameRunning(ifaceName)
	if !running || err != nil {
		var message string = fmt.Sprintf("couldn't find a running Fabric instance with the name %s", configName)
		handleError(message, errorJson)
	}

	return getInstanceProxy(ifaceName)
}

func getRunningInstances() ([]string, error) {
	names, err := getDBusNames()
	if err != nil {
		return nil, err
	}

	filteredNames := []string{}
	for _, name := range names {
		if strings.HasPrefix(name, FABRIC_DBUS_INTERFACE_NAME) {
			filteredNames = append(filteredNames, name)
		}
	}

	return filteredNames, nil
}

func listAll(ctx *cli.Context) error {
	json := ctx.Bool("json")

	filteredNames, err := getRunningInstances()
	if err != nil {
		return err
	}

	if json {
		fmt.Println(serializeData(map[string][]string{"instances-dbus-names": filteredNames}))
		return nil
	}

	for _, dbusName := range filteredNames {
		configName := strings.TrimPrefix(dbusName, FABRIC_DBUS_INTERFACE_NAME+".")
		proxy, err := getInstanceProxy(dbusName)
		if err != nil {
			return err
		}
		fileProp, err := proxy.GetProperty("org.Fabric.fabric.File")
		if err != nil {
			return err
		}
		fmt.Printf("%s: %s\n", configName, fileProp.Value().(string))
	}
	return nil
}

func listWindows(ctx *cli.Context) error {
	json := ctx.Bool("json")
	instance := getArg(ctx, 0, "instance", json)

	busObject, err := checkAndGetInstanceProxy(instance, json)
	if err != nil {
		return err
	}

	windowsProp, err := busObject.GetProperty("org.Fabric.fabric.Windows")
	if err != nil {
		return err
	}

	windows := windowsProp.Value().(map[string]bool)

	if json {
		type FabricWindow struct {
			Name    string `json:"name"`
			Visible bool   `json:"visible"`
		}

		windowsRepack := []FabricWindow{}
		for k, v := range windows {
			windowsRepack = append(windowsRepack, FabricWindow{Name: k, Visible: v})
		}

		fmt.Println(serializeData(windowsRepack))
		return nil
	}

	for k, v := range windows {
		fmt.Printf("window: %s visible: %v\n", k, v)
	}
	return nil
}

func listActions(ctx *cli.Context) error {
	json := ctx.Bool("json")
	instance := getArg(ctx, 0, "instance", json)

	busObject, err := checkAndGetInstanceProxy(instance, json)
	if err != nil {
		return err
	}

	actionsProp, err := busObject.GetProperty("org.Fabric.fabric.Actions")
	if err != nil {
		return err
	}

	actions := actionsProp.Value().(map[string][]string)

	if json {
		type FabricAction struct {
			Name      string   `json:"name"`
			Arguments []string `json:"arguments"`
		}

		actionsRepack := []FabricAction{}
		for k, v := range actions {
			actionsRepack = append(actionsRepack, FabricAction{Name: k, Arguments: v})
		}

		fmt.Println(serializeData(actionsRepack))
		return nil
	}

	for k, v := range actions {
		fmt.Printf("%s (%v)\n", k, strings.Join(v[:], ","))
	}
	return nil
}

func execute(ctx *cli.Context) error {
	json := ctx.Bool("json")

	instance := getArg(ctx, 0, "instance", json)
	source := getArg(ctx, 1, "source", json)

	busObject, err := checkAndGetInstanceProxy(instance, json)
	if err != nil {
		return err
	}

	var exception string
	err = busObject.Call("org.Fabric.fabric.Execute", 0, source).Store(&exception)
	if err != nil {
		return err
	}

	if json {
		fmt.Println(serializeData(map[string]string{"source": source, "exception": exception}))
		return nil
	}

	if exception != "" {
		fmt.Println("exception: " + exception)
	}

	return nil
}

func evaluate(ctx *cli.Context) error {
	json := ctx.Bool("json")

	instance := getArg(ctx, 0, "instance", json)
	code := getArg(ctx, 1, "code", json)

	busObject, err := checkAndGetInstanceProxy(instance, json)
	if err != nil {
		return err
	}

	var result, exception string
	err = busObject.Call("org.Fabric.fabric.Evaluate", 0, code).Store(&result, &exception)
	if err != nil {
		return err
	}

	if json {
		fmt.Println(serializeData(map[string]string{"code": code, "result": result, "exception": exception}))
		return nil
	}

	if exception != "" {
		fmt.Printf("result: %s\nexception: %s\n", result, exception)
	} else {
		fmt.Println("result: " + result)
	}

	return nil
}

func invokeAction(ctx *cli.Context) error {
	json := ctx.Bool("json")

	instance := getArg(ctx, 0, "instance", json)
	action := getArg(ctx, 1, "action", json)
	actionArgs := ctx.Args().Slice()[2:] // a hack

	busObject, err := checkAndGetInstanceProxy(instance, json)
	if err != nil {
		return err
	}

	var responseErr bool
	var responseMsg string
	err = busObject.Call("org.Fabric.fabric.InvokeAction", 0, action, actionArgs).Store(&responseErr, &responseMsg)
	if err != nil {
		return err
	}

	if json {
		fmt.Println(serializeData(
			struct {
				Error   bool   `json:"error"`
				Message string `json:"message"`
			}{responseErr, responseMsg},
		))
		return nil
	}

	if responseErr {
		fmt.Println("couldn't invoke action\nerror: " + responseMsg)
	}

	fmt.Println("action invoked\nreturn message: " + responseMsg)
	return nil
}

func bakeArgsHelp(argsHelp ...string) string {
	message := "\n\nARGUMENTS:"
	for _, argHelp := range argsHelp {
		message = message + "\n" + "\t " + argHelp
	}
	return message
}

func autocompleteInstance(ctx *cli.Context) {
	if ctx.NArg() > 0 {
		return
	}

	filteredNames, err := getRunningInstances()
	if err != nil {
		fmt.Println("")
	}

	for _, instance := range filteredNames {
		fmt.Println(strings.TrimPrefix(instance, FABRIC_DBUS_INTERFACE_NAME+"."))
	}
}

func autocompleteAction(ctx *cli.Context) {
	if ctx.NArg() < 1 {
		autocompleteInstance(ctx)
		return
	}
	instance := getArg(ctx, 0, "instance", false)

	busObject, err := checkAndGetInstanceProxy(instance, false)
	if err != nil {
		return
	}

	actionsProp, err := busObject.GetProperty("org.Fabric.fabric.Actions")
	if err != nil {
		return
	}

	for k, v := range actionsProp.Value().(map[string][]string) {
		fmt.Printf("%s: (%v)\n", k, strings.Join(v[:], ","))
	}
}

func autocompleteStubs(ctx *cli.Context) {
	files, err := filepath.Glob(GetGirLookupPath() + "/*.gir")
	if err != nil {
		return
	}

	for _, path := range files {
		strippedName := strings.TrimSuffix(filepath.Base(path), ".gir")
		if slices.Contains(ctx.Args().Slice(), strippedName) {
			continue
		}
		fmt.Println(strippedName)
	}
}

func main() {
	instanceHelp := "instance: the name of the instance to execute this command on"
	sourceHelp := "source: python source code to execute"
	codeHelp := "code: python code to execute"
	actionHelp := "action-name: the name of the desired action to run"
	actionArgsHelp := "arguments: optional arguments to pass to the action handler function"
	stubsReposHelp := "repositories: repositories needed for type generation, an example repository name would look like this `Gtk-3.0`, the syntax is `<NAME>-<VERSION>`."

	jsonFlag := &cli.BoolFlag{
		Name:    "json",
		Usage:   "to return the output in json format",
		Aliases: []string{"j"},
	}

	app := &cli.App{
		Name:    "fabric-cli",
		Usage:   "an alternative cli for fabric",
		Version: "0.0.2",
		Commands: []*cli.Command{
			{
				Name:    "list-all",
				Usage:   "list all currently running fabric instances",
				Aliases: []string{"la"},
				Flags:   []cli.Flag{jsonFlag},
				Args:    false,
				Action:  listAll,
			},
			{
				Name:         "list-windows",
				Usage:        "list all windows within a running fabric instance",
				Aliases:      []string{"lw"},
				Flags:        []cli.Flag{jsonFlag},
				Args:         true,
				ArgsUsage:    bakeArgsHelp(instanceHelp),
				BashComplete: autocompleteInstance,
				Action:       listWindows,
			},
			{
				Name:         "list-actions",
				Usage:        "list actions within a running fabric instance",
				Aliases:      []string{"actions"},
				Flags:        []cli.Flag{jsonFlag},
				Args:         true,
				ArgsUsage:    bakeArgsHelp(instanceHelp),
				BashComplete: autocompleteInstance,
				Action:       listActions,
			},
			{
				Name:         "execute",
				Usage:        "execute Python code within a running fabric instance",
				Aliases:      []string{"exec"},
				Flags:        []cli.Flag{jsonFlag},
				Args:         true,
				ArgsUsage:    bakeArgsHelp(instanceHelp, sourceHelp),
				BashComplete: autocompleteInstance,
				Action:       execute,
			},
			{
				Name:         "evaluate",
				Usage:        "evaluate Python expression within a running fabric instance and return the result",
				Aliases:      []string{"eval"},
				Flags:        []cli.Flag{jsonFlag},
				Args:         true,
				ArgsUsage:    bakeArgsHelp(instanceHelp, codeHelp),
				BashComplete: autocompleteInstance,
				Action:       evaluate,
			},
			{
				Name:         "invoke-action",
				Usage:        "invoke an action within a running fabric instance",
				Aliases:      []string{"ia", "invoke", "action"},
				Flags:        []cli.Flag{jsonFlag},
				Args:         true,
				ArgsUsage:    bakeArgsHelp(instanceHelp, actionHelp, actionArgsHelp),
				BashComplete: autocompleteAction,
				Action:       invokeAction,
			},
			{
				Name:    "generate-stubs",
				Usage:   "generate a stubs package for getting proper LSP typing on pygobject",
				Aliases: []string{"gs", "stubs", "types"},
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "no-deps",
						Usage: "do not resolve dependencies for the given list of repositories (why?)",
					},
					&cli.PathFlag{
						Name:    "output",
						Usage:   "path in where to save the generated stubs package, defualts to the sitepackages of the active python installation",
						Aliases: []string{"o"},
					},
				},
				Args:         true,
				ArgsUsage:    bakeArgsHelp(stubsReposHelp),
				BashComplete: autocompleteStubs,
				Action: func(ctx *cli.Context) error {
					noDeps := ctx.Bool("no-deps")
					output := ctx.Path("output")
					if len(strings.TrimSpace(output)) == 0 {
						rawOut, err := exec.Command("python", "-c", `import site, os; sp = site.getsitepackages()[0]; print(sp if os.access(sp, os.W_OK | os.X_OK) else site.getusersitepackages(), end='')`).Output()
						if err != nil {
							panic(err)
						}
						output = string(rawOut)
					}
					fmt.Printf("types will be saved to: %s", output)

					GenerateStubs(ctx.Args().Slice(), output+"/gi-stubs", noDeps)
					return nil
				},
			},
		},
		Suggest:              true,
		EnableBashCompletion: true,
		ExitErrHandler:       func(ctx *cli.Context, err error) {},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
