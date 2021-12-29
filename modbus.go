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

	for _, block := range modbusConfig.Lwz {
		switch block.Type {
		case INPUT_REGISTER:
			log.Printf("Reading input registers from %d for %d entries", block.Offset, block.Entries)
			results, err := client.ReadInputRegisters(block.Offset, block.Entries)
			if err != nil {
				log.Fatal(err)
			}
			processModbusRegister(block, results)
		case HOLDING_REGISTER:
			log.Printf("Reading holding registers from %d for %d entries", block.Offset, block.Entries)
			results, err := client.ReadHoldingRegisters(block.Offset, block.Entries)
			if err != nil {
				log.Fatal(err)
			}
			processModbusRegister(block, results)
		default:
			log.Warnf("Did not recognize type of block %s, skipping", block.Type)
		}
	}
}

func processModbusRegister(block ModbusConfigBlock, results []byte) {
	for _, element := range block.Data {
		resultIndex := (element.Address - 1) * 2
		rawValue := float64(binary.BigEndian.Uint16(results[resultIndex : resultIndex+2]))
		log.Debugf("rawValue for %s is %f, abs is %f", element.Name, rawValue, math.Abs(rawValue))

		// skip HK2 if intended
		if element.Labels["hk"] == "2" && options.SkipCircuit2 {
			continue
		}

		// skip "unset" values
		if math.Abs(rawValue) == 32768 {
			continue
		}

		// merge linked data (separation of MWh and kWh)
		if element.MergeData.Address > 0 {
			log.Debugf("Merging address %d into %d for %s", element.MergeData.Address, element.Address, element.Name)
			mergeIndex := (element.MergeData.Address - 1) * 2
			mergeValue := float64(binary.BigEndian.Uint16(results[mergeIndex : mergeIndex+2]))
			if math.Abs(mergeValue) == 32768 {
				continue
			}
			rawValue += mergeValue * float64(element.MergeData.Factor)
		}

		switch element.Type {
		case TYPE_2:
			processedValue := float64(rawValue) / 10
			log.Infof("%s: %.1f %s", element.Name, processedValue, element.Unit)
			createOrRetrieve(element.Name, element.Unit, element.Labels).Set(processedValue)
			valuesMap[element.Name] = append(valuesMap[element.Name], IsgValue{Value: processedValue, Unit: element.Unit, Labels: element.Labels})
		case TYPE_6:
			log.Infof("%s: %.0f %s", element.Name, rawValue, element.Unit)
			createOrRetrieve(element.Name, element.Unit, element.Labels).Set(float64(rawValue))
			valuesMap[element.Name] = append(valuesMap[element.Name], IsgValue{Value: rawValue, Unit: element.Unit, Labels: element.Labels})
		case TYPE_7:
			processedValue := float64(rawValue) / 100
			log.Infof("%s: %.2f %s", element.Name, processedValue, element.Unit)
			createOrRetrieve(element.Name, element.Unit, element.Labels).Set(processedValue)
			valuesMap[element.Name] = append(valuesMap[element.Name], IsgValue{Value: processedValue, Unit: element.Unit, Labels: element.Labels})
		case TYPE_8:
			log.Infof("%s: %s", element.Name, element.Mapping[int(rawValue)])
			labels := element.Labels
			if labels == nil {
				labels = make(map[string]string)
			}
			labels["name"] = strings.ToLower(element.Mapping[int(rawValue)])
			createOrRetrieve(element.Name, element.Unit, labels).Set(1)
			valuesMap[element.Name+"."+labels["name"]] = append(valuesMap[element.Name+"."+labels["name"]], IsgValue{Value: 1})
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
				valuesMap[element.Name+"."+labels["name"]] = append(valuesMap[element.Name+"."+labels["name"]], IsgValue{Value: 1})
			}
		default:
			log.Warnf("%s: Unrecognized type %d", element.Name, element.Type)
		}
	}
}
