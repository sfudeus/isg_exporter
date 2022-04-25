package main

import (
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"math"
	"strings"

	"github.com/pashi-corp/modbus"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

type ModbusConfig struct {
	Lwz []ModbusConfigBlock
}

const (
	INPUT_REGISTER   string = "INPUT_REGISTER"
	HOLDING_REGISTER string = "HOLDING_REGISTER"
	MODBUS_PORT      int    = 502
)

const (
	TYPE_2    int = 2
	TYPE_6    int = 6
	TYPE_7    int = 7
	TYPE_8    int = 8
	BITVECTOR int = 100
)

type ModbusConfigBlock struct {
	Type    string
	Offset  uint16
	Entries uint16
	Data    []ModbusConfigElement
}

type ModbusConfigElement struct {
	Name      string
	Address   uint16
	Type      int
	Unit      string
	MergeData ModbusConfigElementMergeData `yaml:"mergeData"`
	Labels    map[string]string
	Mapping   map[int]string
}

type ModbusConfigElementMergeData struct {
	Address uint16
	Factor  int
}

var client modbus.Client
var modbusConfig ModbusConfig

func prepareModbus() {

	modbusConfigFile, err := ioutil.ReadFile("modbus-mapping.yaml")
	if err != nil {
		log.Fatal(err)
	}
	modbusConfig = ModbusConfig{}
	err = yaml.Unmarshal(modbusConfigFile, &modbusConfig)
	if err != nil {
		log.Fatal(err)
	}

	// Modbus TCP
	modbusHostname := strings.TrimPrefix(options.URL, "http://")
	handler := modbus.NewTCPClientHandler(modbusHostname + ":" + fmt.Sprint(MODBUS_PORT))
	handler.SlaveId = byte(options.ModbusSlaveId)

	client = modbus.NewClient(handler)
}

func gatherModbusData() {

	newValuesMap := make(map[string][]IsgValue)

	for _, block := range modbusConfig.Lwz {
		switch block.Type {
		case INPUT_REGISTER:
			log.Printf("Reading input registers from %d for %d entries", block.Offset, block.Entries)
			results, err := client.ReadInputRegisters(block.Offset, block.Entries)
			if err != nil {
				log.Fatal(err)
			}
			processModbusRegister(block, results, newValuesMap)
		case HOLDING_REGISTER:
			log.Printf("Reading holding registers from %d for %d entries", block.Offset, block.Entries)
			results, err := client.ReadHoldingRegisters(block.Offset, block.Entries)
			if err != nil {
				log.Fatal(err)
			}
			processModbusRegister(block, results, newValuesMap)
		default:
			log.Warnf("Did not recognize type of block %s, skipping", block.Type)
		}
	}

	valuesMap = newValuesMap
}

func readNumericEntry(element ModbusConfigElement, results []byte) (uint16, float64) {

	resultIndex := (element.Address - 1) * 2
	rawValue := binary.BigEndian.Uint16(results[resultIndex : resultIndex+2])
	log.Debugf("rawValue for %s is %d", element.Name, rawValue)

	var processedValue float64

	switch element.Type {
	case TYPE_2:
		processedValue = float64(int16(rawValue)) / 10
	case TYPE_6:
		processedValue = float64(rawValue)
		// merge linked data (separation of MWh and kWh)
		if element.MergeData.Address > 0 {
			log.Debugf("Merging address %d into %d for %s", element.MergeData.Address, element.Address, element.Name)
			mergeIndex := (element.MergeData.Address - 1) * 2
			mergeValue := float64(binary.BigEndian.Uint16(results[mergeIndex : mergeIndex+2]))
			if math.Abs(mergeValue) != 32768 {
				processedValue += (mergeValue * float64(element.MergeData.Factor))
			}
		}
	case TYPE_7:
		processedValue = float64(int16(rawValue)) / 100
	case TYPE_8, BITVECTOR:
		// no processed value for those types
	default:
		log.Warnf("%s: Unrecognized numeric type %d", element.Name, element.Type)
	}

	log.Debugf("processedValue for %s is %f, abs is %f", element.Name, processedValue, math.Abs(processedValue))
	return rawValue, processedValue
}

func processModbusRegister(block ModbusConfigBlock, results []byte, newValuesMap map[string][]IsgValue) {
	for _, element := range block.Data {

		// skip HK2 if intended
		if element.Labels["hk"] == "2" && options.SkipCircuit2 {
			continue
		}

		rawValue, processedValue := readNumericEntry(element, results)

		// skip "unset" values
		if math.Abs(float64(rawValue)) == 32768 {
			continue
		}

		switch element.Type {
		case TYPE_2, TYPE_6, TYPE_7:
			log.Infof("%s: %.1f %s", element.Name, processedValue, element.Unit)
			createOrRetrieve(element.Name, element.Unit, element.Labels).Set(processedValue)
			newValuesMap[element.Name] = append(newValuesMap[element.Name], IsgValue{Value: processedValue, Unit: element.Unit, Labels: element.Labels})
		case TYPE_8:
			log.Infof("%s: %s", element.Name, element.Mapping[int(rawValue)])
			labels := element.Labels
			if labels == nil {
				labels = make(map[string]string)
			}
			for mappingKey, mappingName := range element.Mapping {
				labels["name"] = strings.ToLower(mappingName)
				status := 0
				if mappingKey == int(rawValue) {
					status = 1
				}
				createOrRetrieve(element.Name, element.Unit, labels).Set(float64(status))
				newValuesMap[element.Name+"."+labels["name"]] = append(newValuesMap[element.Name+"."+labels["name"]], IsgValue{Value: 1})
			}
		case BITVECTOR:
			log.Infof("%s:", element.Name)
			log.Debugf("results[0]: %b", results[0])
			log.Debugf("results[1]: %b", results[1])
			for bit, name := range element.Mapping {
				shift := (bit - 1) % 8
				resultsIndex := 1 - (bit-1)/8
				log.Debugf("Looking into results[%d], shift %d for %s", resultsIndex, shift, name)
				value := results[resultsIndex] >> shift & 1
				log.Infof("%s is %b", name, value)

				labels := element.Labels
				if labels == nil {
					labels = make(map[string]string)
				}
				labels["name"] = strings.ToLower(name)
				createOrRetrieve("flag", "", labels).Set(float64(value))
				newValuesMap[element.Name+"."+labels["name"]] = append(newValuesMap[element.Name+"."+labels["name"]], IsgValue{Value: 1})
			}
		default:
			log.Warnf("%s: Unrecognized type %d", element.Name, element.Type)
		}
	}
}

type SGReady struct {
	name string
	data []byte
}

const (
	Lock   = "lock"
	Normal = "normal"
	Active = "active"
	Force  = "force"
)

func SetSGReadyLevel(level string) {

	var sgready_target SGReady

	switch level {
	case Lock:
		sgready_target = SGReady{Lock, []byte{0, 0, 0, 1}}
	case Normal:
		sgready_target = SGReady{Normal, []byte{0, 0, 0, 0}}
	case Active:
		sgready_target = SGReady{Active, []byte{0, 1, 0, 0}}
	case Force:
		sgready_target = SGReady{Force, []byte{0, 1, 0, 1}}
	default:
		log.Fatalf("Unexpected level %s", level)
	}

	log.Debugf("Setting sgready to %b", sgready_target.data)
	result, err := client.WriteMultipleRegisters(4001, 4, sgready_target.data)
	if err != nil {
		log.Fatal("Failed to set sg-ready flags\n", err)
	}

	result, err = client.ReadHoldingRegisters(4001, 2)
	for i := range [2]int{} {
		log.Infof("Successfully read back %d: %d", 4001+i, binary.BigEndian.Uint16(result[i*2:i*2+2]))
	}

	result, err = client.ReadInputRegisters(5000, 1)
	log.Infof("Successfully read 5001: %d", binary.BigEndian.Uint16(result[0:2]))
}
