package homebase

type ClusterDef struct {
	Name       string
	HAPService string
	Commands   []ClusterCommand
	Attributes []ClusterAttribute
}

type ClusterCommand struct {
	Name      string
	Arguments []CommandArgument
}

type CommandArgument struct {
	Name     string
	Type     string
	Required bool
}

type ClusterAttribute struct {
	Name     string
	Type     string
	Writable bool
}

var SupportedClusters = map[string]ClusterDef{
	"OnOff": {
		Name:       "OnOff",
		HAPService: "Switch",
		Commands: []ClusterCommand{
			{Name: "On"},
			{Name: "Off"},
			{Name: "Toggle"},
		},
		Attributes: []ClusterAttribute{
			{Name: "OnOff", Type: "bool", Writable: false},
		},
	},
	"LevelControl": {
		Name:       "LevelControl",
		HAPService: "Lightbulb",
		Commands: []ClusterCommand{
			{Name: "MoveToLevel", Arguments: []CommandArgument{
				{Name: "level", Type: "uint8", Required: true},
				{Name: "transition_time", Type: "uint16", Required: false},
			}},
		},
		Attributes: []ClusterAttribute{
			{Name: "CurrentLevel", Type: "uint8", Writable: false},
			{Name: "MinLevel", Type: "uint8", Writable: false},
			{Name: "MaxLevel", Type: "uint8", Writable: false},
		},
	},
	"ColorControl": {
		Name:       "ColorControl",
		HAPService: "Lightbulb",
		Commands: []ClusterCommand{
			{Name: "MoveToHueAndSaturation", Arguments: []CommandArgument{
				{Name: "hue", Type: "uint8", Required: true},
				{Name: "saturation", Type: "uint8", Required: true},
				{Name: "transition_time", Type: "uint16", Required: false},
			}},
			{Name: "MoveToColorTemperature", Arguments: []CommandArgument{
				{Name: "color_temperature_mireds", Type: "uint16", Required: true},
				{Name: "transition_time", Type: "uint16", Required: false},
			}},
		},
		Attributes: []ClusterAttribute{
			{Name: "CurrentHue", Type: "uint8", Writable: false},
			{Name: "CurrentSaturation", Type: "uint8", Writable: false},
			{Name: "ColorTemperatureMireds", Type: "uint16", Writable: false},
		},
	},
	"Thermostat": {
		Name:       "Thermostat",
		HAPService: "Thermostat",
		Commands: []ClusterCommand{
			{Name: "SetpointRaiseLower", Arguments: []CommandArgument{
				{Name: "mode", Type: "uint8", Required: true},
				{Name: "amount", Type: "int8", Required: true},
			}},
		},
		Attributes: []ClusterAttribute{
			{Name: "LocalTemperature", Type: "int16", Writable: false},
			{Name: "OccupiedHeatingSetpoint", Type: "int16", Writable: true},
			{Name: "OccupiedCoolingSetpoint", Type: "int16", Writable: true},
			{Name: "SystemMode", Type: "uint8", Writable: true},
		},
	},
	"DoorLock": {
		Name:       "DoorLock",
		HAPService: "LockMechanism",
		Commands: []ClusterCommand{
			{Name: "LockDoor"},
			{Name: "UnlockDoor"},
		},
		Attributes: []ClusterAttribute{
			{Name: "LockState", Type: "uint8", Writable: false},
			{Name: "LockType", Type: "uint8", Writable: false},
		},
	},
	"WindowCovering": {
		Name:       "WindowCovering",
		HAPService: "WindowCovering",
		Commands: []ClusterCommand{
			{Name: "UpOrOpen"},
			{Name: "DownOrClose"},
			{Name: "StopMotion"},
			{Name: "GoToLiftPercentage", Arguments: []CommandArgument{
				{Name: "lift_percent_100ths", Type: "uint16", Required: true},
			}},
		},
		Attributes: []ClusterAttribute{
			{Name: "CurrentPositionLiftPercent100ths", Type: "uint16", Writable: false},
			{Name: "TargetPositionLiftPercent100ths", Type: "uint16", Writable: false},
		},
	},
	"FanControl": {
		Name:       "FanControl",
		HAPService: "Fan",
		Commands:   []ClusterCommand{},
		Attributes: []ClusterAttribute{
			{Name: "FanMode", Type: "uint8", Writable: true},
			{Name: "PercentSetting", Type: "uint8", Writable: true},
			{Name: "SpeedSetting", Type: "uint8", Writable: true},
		},
	},
	"OccupancySensing": {
		Name:       "OccupancySensing",
		HAPService: "OccupancySensor",
		Commands:   []ClusterCommand{},
		Attributes: []ClusterAttribute{
			{Name: "Occupancy", Type: "uint8", Writable: false},
			{Name: "OccupancySensorType", Type: "uint8", Writable: false},
		},
	},
	"TemperatureMeasurement": {
		Name:       "TemperatureMeasurement",
		HAPService: "TemperatureSensor",
		Commands:   []ClusterCommand{},
		Attributes: []ClusterAttribute{
			{Name: "MeasuredValue", Type: "int16", Writable: false},
			{Name: "MinMeasuredValue", Type: "int16", Writable: false},
			{Name: "MaxMeasuredValue", Type: "int16", Writable: false},
		},
	},
	"RelativeHumidityMeasurement": {
		Name:       "RelativeHumidityMeasurement",
		HAPService: "HumiditySensor",
		Commands:   []ClusterCommand{},
		Attributes: []ClusterAttribute{
			{Name: "MeasuredValue", Type: "uint16", Writable: false},
		},
	},
}

func ClusterDefFor(name string) (ClusterDef, bool) {
	def, ok := SupportedClusters[name]
	return def, ok
}

func DeviceTypeFromClusters(clusters []string) string {
	has := make(map[string]bool, len(clusters))
	for _, c := range clusters {
		has[c] = true
	}

	if has["Thermostat"] {
		return "thermostat"
	}
	if has["DoorLock"] {
		return "lock"
	}
	if has["WindowCovering"] {
		return "window_covering"
	}
	if has["FanControl"] {
		return "fan"
	}
	if has["ColorControl"] {
		return "color_light"
	}
	if has["LevelControl"] {
		return "dimmable_light"
	}
	if has["OccupancySensing"] {
		return "occupancy_sensor"
	}
	if has["TemperatureMeasurement"] {
		return "temperature_sensor"
	}
	if has["RelativeHumidityMeasurement"] {
		return "humidity_sensor"
	}
	if has["OnOff"] {
		return "switch"
	}
	return "unknown"
}
