package main

import (
	"encoding/binary"
	"io/ioutil"
	"log"
	"math"
	"os"
	"strings"

	"github.com/pashi-corp/modbus"
	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/yaml.v2"
)

type ModbusConfig struct {
	Lwz []ModbusConfigBlock
}

const (
	INPUT_REGISTER   string = "INPUT_REGISTER"
	HOLDING_REGISTER string = "HOLDING_REGISTER"
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
	Name    string
	Address uint16
	Type    int
	Unit    string
	Mapping map[int]string
}

var client modbus.Client
var modbusConfig ModbusConfig

func prepareModbus() {

	modbusConfigFile, err := ioutil.ReadFile("modbus-mapping.yaml")
	if err != nil {
		log.Println(err)
		os.Exit(2)
	}
	modbusConfig = ModbusConfig{}
	err = yaml.Unmarshal(modbusConfigFile, &modbusConfig)
	if err != nil {
		log.Fatal(err)
		os.Exit(2)
	}

	// Modbus TCP
	modbusHostname := strings.TrimLeft(options.URL, "http://")
	handler := modbus.NewTCPClientHandler(modbusHostname + ":502")
	handler.SlaveId = 0x01

	client = modbus.NewClient(handler)
}

func gatherModbusData(flagRemovalList map[string]prometheus.Gauge) {

	for _, block := range modbusConfig.Lwz {
		switch block.Type {
		case INPUT_REGISTER:
			log.Printf("Reading input registers from %d for %d entries", block.Offset, block.Entries)
			results, err := client.ReadInputRegisters(block.Offset, block.Entries)
			if err != nil {
				log.Fatal(err)
			}
			processModbusRegister(block, results, flagRemovalList)
		case HOLDING_REGISTER:
			log.Printf("Reading holding registers from %d for %d entries", block.Offset, block.Entries)
			results, err := client.ReadHoldingRegisters(block.Offset, block.Entries)
			if err != nil {
				log.Fatal(err)
			}
			processModbusRegister(block, results, flagRemovalList)
		default:
			log.Printf("Did not recognize type of block %s, skipping", block.Type)
		}
	}

}

func processModbusRegister(block ModbusConfigBlock, results []byte, flagRemovalList map[string]prometheus.Gauge) {
	for _, element := range block.Data {
		resultIndex := (element.Address - 1) * 2
		rawValue := int16(binary.BigEndian.Uint16(results[resultIndex : resultIndex+2]))
		if math.Abs(float64(rawValue)) == 32768 {
			continue
		}
		switch element.Type {
		case TYPE_2:
			processedValue := float64(rawValue) / 10
			log.Printf("%s: %.1f %s", element.Name, processedValue, element.Unit)
			createOrRetrieve(element.Name, element.Unit).Set(processedValue)
		case TYPE_6:
			log.Printf("%s: %d %s", element.Name, rawValue, element.Unit)
			createOrRetrieve(element.Name, element.Unit).Set(float64(rawValue))
		case TYPE_7:
			processedValue := float64(rawValue) / 100
			log.Printf("%s: %.2f %s", element.Name, processedValue, element.Unit)
			createOrRetrieve(element.Name, element.Unit).Set(processedValue)
		case TYPE_8:
			log.Printf("%s: %s", element.Name, element.Mapping[int(rawValue)])
			createOrRetrieve(element.Name, element.Unit).Set(float64(rawValue))
		case BITVECTOR:
			log.Printf("%s:", element.Name)
			log.Printf("results[0]: %b", results[0])
			log.Printf("results[1]: %b", results[1])
			for bit, name := range element.Mapping {
				shift := (bit - 1) % 8
				resultsIndex := 1 - (bit-1)/8
				//log.Printf("Looking into results[%d], shift %d for %s", resultsIndex, shift, name)
				value := results[resultsIndex] >> shift & 1
				log.Printf("%s is %b", name, value)

				label := "flag_" + name
				flagGauge := createOrRetrieve(label, "")
				flagGauge.Set(1)
				flagGaugesMap[label] = flagGauge
				delete(flagRemovalList, label)
			}
		default:
			log.Printf("%s: Unrecognized type %d", element.Name, element.Type)
		}
	}
}
