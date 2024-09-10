package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
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
	// urfave's cli library doesn't support named arguments (or does it?).
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

	for k, v := range windowsProp.Value().(map[string]bool) {
		fmt.Printf("Window: %s Visible: %v\n", k, v)
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

func main() {
	instanceHelp := "instance: the name of the instance to execute this command on"
	sourceHelp := "source: python source code to execute"
	codeHelp := "code: python code to execute"

	jsonFlag := &cli.BoolFlag{
		Name:    "json",
		Usage:   "to return the output in json format",
		Aliases: []string{"j"},
	}

	app := &cli.App{
		Name:    "fabric-cli",
		Usage:   "an alternative cli for fabric",
		Version: "0.0.2 (Unreleased)",
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
				Name:         "execute",
				Usage:        "execute Python code within a running fabric instance instance",
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
		},
		Suggest:              true,
		EnableBashCompletion: true,
		ExitErrHandler:       func(ctx *cli.Context, err error) {},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
