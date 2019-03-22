/******************************************************
# DESC    :  tcp 客户端数据采集
# AUTHOR  : Lwdsun
# EMAIL   : lwdsun@sina.com
# MOD     : 2018-06-05
            2018-07-23 增加6路温度的数据采集, 原16路温度的数据采集由10+6变13+3的采集方式
# FILE    : client.go
******************************************************/
package electrictcp

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"runtime"
	"time"

	"github.com/eurake/acpm/model"
	"github.com/eurake/xutils"
	"github.com/eurake/xutils/crc"
	"github.com/robfig/cron"
)

type Client struct {
	conn   net.Conn
	closed int32
	uuid   string

	isAuth   bool
	online   bool
	rightNow bool

	currentGather string // 当前采集的仪表
	tm            time.Time
	phoneNum      string
	command       GatherCommand
	devies        []model.EDevice
	currentDevice model.EDevice
}

type GatherCommand struct {
	ModbusAddress  byte
	Functions      byte
	ReadDataLength int
	Writes         []byte
	ModelType      string
}

func NewClient(conn net.Conn) *Client {
	client := new(Client)
	//初始化Connection
	client.conn = conn // conn is net.Conn or engineio.Conn
	client.isAuth = false
	client.rightNow = true
	client.uuid = xutils.UuidStr()
	Conns.Add(client)
	return client
}

func (client *Client) Read() {
	for {
		if client.isAuth {
			err := client.ProcessData()
			if err != nil {
				break
			}
		} else {
			err := client.HandleAuth()
			if err != nil {
				break
			}
		}
		runtime.Gosched()
	}
}

func (client *Client) ProcessData() error {
	client.conn.SetReadDeadline(time.Now().Add(60 * 6 * time.Second))
	buff := make([]byte, 256)
	fmt.Println("processdata进入读中")
	count, err := client.conn.Read(buff)
	fmt.Println("processdat读取完成: 数据长度( " + xutils.ToString(count))
	if err != nil {
		AuthConns.Remove(client.phoneNum)
		client.conn.Close()
		return err
	}
	if count == 2 || count == 0 || count == 16 {
		// 判断为心跳包
		fmt.Println("心跳包")
	}
	// 解析数据
	client.ProcessProtocol(buff[0:count])
	return nil
}

func (client *Client) HandleAuth() error {
	client.conn.SetReadDeadline(time.Now().Add(60 * 6 * time.Second))
	buff := make([]byte, 512)
	fmt.Println("进入读中")
	count, err := client.conn.Read(buff)
	fmt.Println("读取完成")
	if err != nil {
		Conns.Remove(client.uuid)
		return err
	}
	if count >= 13 {
		// 转成string
		phoneNum := string(buff[0:13])
		fmt.Println("测试")
		fmt.Println(phoneNum)
		var netWork model.NetWork
		err := model.DB.Where("phone_num = ?", phoneNum).First(&netWork).Error
		if err != nil {
			Conns.Remove(client.uuid)
			// 不立刻关闭, 防止客户端不停的重连
			// fmt.P
			time.Sleep(2 * time.Second)
			client.conn.Close()
			return errors.New("not auth ++++++++++++++++++")
		}
		netWork.State = true
		model.DB.Save(&netWork)

		client.isAuth = true
		client.phoneNum = phoneNum
		client.rightNow = true
		Conns.Remove(client.uuid)
		AuthConns.Add(client)
		var devices []model.EDevice
		err = model.DB.Where("net_work_id = ?", netWork.ID).Find(&devices).Error
		if err != nil {
			// 表示没有
		} else {
			for i, _ := range devices {
				// 排除不需要数据采集的Modbus
				if devices[i].ModbusAddress != 0 {
					client.devies = append(client.devies, devices[i])
				}
			}
		}
		if client.rightNow {
			go client.ProcessSend()
			go client.CronTask()
		}
	}
	return nil
}

func (client *Client) ProcessProtocol(buf []byte) {
	//
	length := len(buf)
	if length < 7 {
		return
	}
	//fmt.Println(length, buf)
	// CRC
	// 地址
	if client.command.ModbusAddress == buf[0] && length == client.command.ReadDataLength {
		if client.command.ModelType == "0001" {
			if length == 21 {
				// PMW200
				ia := binary.BigEndian.Uint32(buf[3:7])
				ib := binary.BigEndian.Uint32(buf[7:11])
				ic := binary.BigEndian.Uint32(buf[11:15])
				fmt.Println("ia/ib/ic")
				fmt.Println(ia, ib, ic)
				fa := float64(ia) * 0.01
				fb := float64(ib) * 0.01
				fc := float64(ic) * 0.01
				ProcessAlarmAndSave(client.currentDevice.ID, "ia", xutils.Decimal(fa), "电流")
				ProcessAlarmAndSave(client.currentDevice.ID, "ib", xutils.Decimal(fb), "电流")
				ProcessAlarmAndSave(client.currentDevice.ID, "ic", xutils.Decimal(fc), "电流")
			} else if length == 29 {
				// PMW200
				ua := binary.BigEndian.Uint32(buf[3:7])
				ub := binary.BigEndian.Uint32(buf[7:11])
				uc := binary.BigEndian.Uint32(buf[11:15])
				fmt.Println("ua/ub/uc")
				fmt.Println(ua, ub, uc)
				fua := float64(ua) * 0.1
				fub := float64(ub) * 0.1
				fuc := float64(uc) * 0.1
				ProcessAlarmAndSave(client.currentDevice.ID, "ua", xutils.Decimal(fua), "电压")
				ProcessAlarmAndSave(client.currentDevice.ID, "ub", xutils.Decimal(fub), "电压")
				ProcessAlarmAndSave(client.currentDevice.ID, "uc", xutils.Decimal(fuc), "电压")
			} else if length == 7 {
				// PMW200 功率因素
				hz := binary.BigEndian.Uint16(buf[3:5])
				fmt.Println("cosφ")
				fhz := float64(hz) * 0.001
				ProcessAlarmAndSave(client.currentDevice.ID, "cos", xutils.Decimal(fhz), "")
			} else if length == 13 {
				// PMW200 有功能电能
				hz := binary.BigEndian.Uint64(buf[3:11])
				fhz := float64(hz) * 0.00001
				ProcessAlarmAndSave(client.currentDevice.ID, "p", xutils.Decimal(fhz), "")
			}
		}
		if client.command.ModelType == "0002" {
			if length == 7 {
				// 烟感
				fmt.Println("-----------------------------------------------------------------------")
				sm := binary.BigEndian.Uint16(buf[3:5])
				fsm := float64(sm) * 1.0
				ProcessAlarmAndSave(client.currentDevice.ID, "sm", xutils.Decimal(fsm), "烟感")
			}
		}
		if client.command.ModelType == "0003" {
			if length == 37 {
				// 16开关量输入
				for j := 0; j < 16; j++ {
					filed := "k" + xutils.ToString(j+1)
					nv := binary.BigEndian.Uint16(buf[3+2*j : 5+2*j])
					fnv := float64(nv) * 1.0
					ProcessAlarmAndSave(client.currentDevice.ID, filed, xutils.Decimal(fnv), "开关")
				}
			}
		}
		if client.command.ModelType == "0004" {
			// 8路开关量输入
			if length == 21 {
				for j := 0; j < 8; j++ {
					filed := "k" + xutils.ToString(j+1)
					nv := binary.BigEndian.Uint16(buf[3+2*j : 5+2*j])
					fnv := float64(nv) * 1.0
					ProcessAlarmAndSave(client.currentDevice.ID, filed, xutils.Decimal(fnv), "开关")
				}
			}
		}
		if client.command.ModelType == "0005" {
			if length == 27 {
				// 8 交流
				for j := 0; j < 8; j++ {
					filed := "i" + xutils.ToString(j+1)
					niv := binary.BigEndian.Uint16(buf[3+2*j : 5+2*j])
					fiv := float64(niv) * 0.01
					ProcessAlarmAndSave(client.currentDevice.ID, filed, xutils.Decimal(fiv), "电流")
				}
			}
		}
		if client.command.ModelType == "0006" {
			if length == 37 {
				// 16 温度 1-16
				for j := 0; j < 16; j++ {
					filed := "t" + xutils.ToString(j+1)
					ntv := binary.BigEndian.Uint16(buf[3+2*j : 5+2*j])
					ftv := float64(ntv) * 0.01
					ProcessAlarmAndSave(client.currentDevice.ID, filed, xutils.Decimal(ftv), "温度")
				}
			}
		}
		if client.command.ModelType == "0007" {
			// 8路温度
			if length == 21 {
				for j := 0; j < 8; j++ {
					filed := "t" + xutils.ToString(j+1)
					ntv := binary.BigEndian.Uint16(buf[3+2*j : 5+2*j])
					ftv := float64(ntv) * 0.01
					ProcessAlarmAndSave(client.currentDevice.ID, filed, xutils.Decimal(ftv), "温度")
				}
			}
		}
		if client.command.ModelType == "0008" {
			if length == 17 {
				// 6路温度
				for j := 0; j < 6; j++ {
					filed := "t" + xutils.ToString(j+1)
					ntv := binary.BigEndian.Uint16(buf[3+2*j : 5+2*j])
					ftv := float64(ntv) * 0.01
					ProcessAlarmAndSave(client.currentDevice.ID, filed, xutils.Decimal(ftv), "温度")
				}
			}
		}
		if client.command.ModelType == "0009" {
			if length == 17 {
				// DTSD1352 电流电压 6 + 1, 7 * 2 + 5 = 19
				ua := binary.BigEndian.Uint16(buf[3:5])
				ub := binary.BigEndian.Uint16(buf[5:7])
				uc := binary.BigEndian.Uint16(buf[7:9])
				fmt.Println("ua/ub/uc")
				fmt.Println(ua, ub, uc)
				fua := float64(ua) * 0.1
				fub := float64(ub) * 0.1
				fuc := float64(uc) * 0.1
				if client.currentDevice.ID == 54 {
					ProcessAlarmAndSave(client.currentDevice.ID, "ua", xutils.Decimal(fua*0.1), "电压")
					ProcessAlarmAndSave(client.currentDevice.ID, "ub", xutils.Decimal(fub*0.1), "电压")
					ProcessAlarmAndSave(client.currentDevice.ID, "uc", xutils.Decimal(fuc*0.1), "电压")
				} else {
					ProcessAlarmAndSave(client.currentDevice.ID, "ua", xutils.Decimal(fua), "电压")
					ProcessAlarmAndSave(client.currentDevice.ID, "ub", xutils.Decimal(fub), "电压")
					ProcessAlarmAndSave(client.currentDevice.ID, "uc", xutils.Decimal(fuc), "电压")
				}

				ia := binary.BigEndian.Uint16(buf[9:11])
				ib := binary.BigEndian.Uint16(buf[11:13])
				ic := binary.BigEndian.Uint16(buf[13:15])
				fmt.Println("ia/ib/ic")
				fmt.Println(ia, ib, ic)
				fa := float64(ia) * 0.01
				fb := float64(ib) * 0.01
				fc := float64(ic) * 0.01
				ProcessAlarmAndSave(client.currentDevice.ID, "ia", xutils.Decimal(fa), "电流")
				ProcessAlarmAndSave(client.currentDevice.ID, "ib", xutils.Decimal(fb), "电流")
				ProcessAlarmAndSave(client.currentDevice.ID, "ic", xutils.Decimal(fc), "电流")
			} else if length == 7 {
				// 总功率因素
				cos := binary.BigEndian.Uint16(buf[3:5])
				fmt.Println("cosφ")
				fcos := float64(cos) * 1.0
				for n := 0; n < 4; n++ {
					if fcos > 1.0 {
						fcos = fcos * 0.1
					} else {
						break
					}
				}
				ProcessAlarmAndSave(client.currentDevice.ID, "cos", xutils.Decimal(fcos), "")
			} else if length == 9 {
				// 总有功功率
				p := binary.BigEndian.Uint32(buf[3:7])
				fmt.Println("p")
				fp := float64(p) * 0.001
				ProcessAlarmAndSave(client.currentDevice.ID, "p", xutils.Decimal(fp), "")
			}
		}
		if client.command.ModelType == "0014" {
			if length == 17 {
				// DTSD1352 电流电压 6 + 1, 7 * 2 + 5 = 19
				ua := binary.BigEndian.Uint16(buf[3:5])
				ub := binary.BigEndian.Uint16(buf[5:7])
				uc := binary.BigEndian.Uint16(buf[7:9])
				fmt.Println("ua/ub/uc")
				fmt.Println(ua, ub, uc)
				fua := float64(ua) * 0.1
				fub := float64(ub) * 0.1
				fuc := float64(uc) * 0.1
				ProcessAlarmAndSave(client.currentDevice.ID, "ua", xutils.Decimal(fua), "电压")
				ProcessAlarmAndSave(client.currentDevice.ID, "ub", xutils.Decimal(fub), "电压")
				ProcessAlarmAndSave(client.currentDevice.ID, "uc", xutils.Decimal(fuc), "电压")

				ia := binary.BigEndian.Uint16(buf[9:11])
				ib := binary.BigEndian.Uint16(buf[11:13])
				ic := binary.BigEndian.Uint16(buf[13:15])
				fmt.Println("ia/ib/ic")
				fmt.Println(ia, ib, ic)
				fa := float64(ia) * 0.01
				fb := float64(ib) * 0.01
				fc := float64(ic) * 0.01
				ProcessAlarmAndSave(client.currentDevice.ID, "ia", xutils.Decimal(fa), "电流")
				ProcessAlarmAndSave(client.currentDevice.ID, "ib", xutils.Decimal(fb), "电流")
				ProcessAlarmAndSave(client.currentDevice.ID, "ic", xutils.Decimal(fc), "电流")
			} else if length == 7 {
				// 总功率因素
				cos := binary.BigEndian.Uint16(buf[3:5])
				fmt.Println("cosφ")
				fcos := float64(cos) * 1.0
				for n := 0; n < 4; n++ {
					if fcos > 1.0 {
						fcos = fcos * 0.1
					} else {
						break
					}
				}
				ProcessAlarmAndSave(client.currentDevice.ID, "cos", xutils.Decimal(fcos), "")
			} else if length == 9 {
				// 总有功功率
				fmt.Println("+============================================================")
				p := binary.BigEndian.Uint32(buf[3:7])
				fmt.Println("p")
				fp := float64(p)
				ProcessAlarmAndSave(client.currentDevice.ID, "p", xutils.Decimal(fp), "")
			}
		}
	}
}

func (client *Client) Write() {
	time.Sleep(10 * 60 * time.Second)
}

func (client *Client) CronTask() {
	c := cron.New()
	fmt.Println("进入CronTask sleep 60 s")
	var spec string
	if len(client.devies) >= 4 {
		spec = "0 0/2 * * * ?"
	} else {
		spec = "0 0/1 * * * ?"
	}
	c.AddFunc(spec, func() {
		fmt.Println("进入CronTask 回调")
		client.ProcessSend()
	})
	c.Start()
}

//
func (client *Client) ProcessSend() {
	if len(client.devies) == 0 {
		fmt.Println("进入发送ProcessSend,没有设备")
		return
	}

	for _, dtemp := range client.devies {
		client.currentDevice = dtemp
		model.DB.Model(&dtemp).Related(&dtemp.Gather)
		commands := GetCommand(dtemp.Gather.CodeNum, uint(dtemp.ModbusAddress))
		for _, command := range commands {
			client.command = command
			client.conn.Write(command.Writes)
			fmt.Println(command.Writes)
			time.Sleep(5 * time.Second)
		}
	}
}

func GetCommand(dtype string, modbus uint) []GatherCommand {
	var gatherCommands []GatherCommand
	if dtype == "0001" {
		// pwm200
		comand := GatherCommand{
			ReadDataLength: 21,
			ModbusAddress:  byte(modbus),
			ModelType:      dtype,
			Writes:         GetCrcCommand(byte(modbus), 0x04, 0x044C, 0x0008),
		}
		gatherCommands = append(gatherCommands, comand)
		comand = GatherCommand{
			ReadDataLength: 29,
			ModbusAddress:  byte(modbus),
			ModelType:      dtype,
			Writes:         GetCrcCommand(byte(modbus), 0x04, 0x0460, 0x000C),
		}
		gatherCommands = append(gatherCommands, comand)
		comand = GatherCommand{
			ReadDataLength: 7,
			ModbusAddress:  byte(modbus),
			ModelType:      dtype,
			Writes:         GetCrcCommand(byte(modbus), 0x04, 0x04A9, 0x0001),
		}
		gatherCommands = append(gatherCommands, comand)
		comand = GatherCommand{
			ReadDataLength: 13,
			ModbusAddress:  byte(modbus),
			ModelType:      dtype,
			Writes:         GetCrcCommand(byte(modbus), 0x04, 0x0480, 0x0004),
		}
		gatherCommands = append(gatherCommands, comand)
	} else if dtype == "0002" {
		// 烟感
		comand := GatherCommand{
			ReadDataLength: 7,
			ModbusAddress:  byte(modbus),
			ModelType:      dtype,
			Writes:         GetCrcCommand(byte(modbus), 0x03, 0x0000, 0x0001),
		}

		gatherCommands = append(gatherCommands, comand)
	} else if dtype == "0003" {
		// 开关量16路
		comand := GatherCommand{
			ReadDataLength: 37,
			ModbusAddress:  byte(modbus),
			ModelType:      dtype,
			Writes:         GetCrcCommand(byte(modbus), 0x03, 0x0300, 0x0010),
		}

		gatherCommands = append(gatherCommands, comand)
	} else if dtype == "0004" {
		// 开关量8路
		comand := GatherCommand{
			ReadDataLength: 21,
			ModbusAddress:  byte(modbus),
			ModelType:      dtype,
			Writes:         GetCrcCommand(byte(modbus), 0x03, 0x0300, 0x0008),
		}
		gatherCommands = append(gatherCommands, comand)
	} else if dtype == "0005" {
		// 8路电流
		comand := GatherCommand{
			ReadDataLength: 27,
			ModbusAddress:  byte(modbus),
			ModelType:      dtype,
			Writes:         GetCrcCommand(byte(modbus), 0x03, 0x0028, 0x000B),
		}

		gatherCommands = append(gatherCommands, comand)
	} else if dtype == "0006" {
		// 温度16路
		comand := GatherCommand{
			ReadDataLength: 37,
			ModbusAddress:  byte(modbus),
			ModelType:      dtype,
			Writes:         GetCrcCommand(byte(modbus), 0x04, 0x0000, 0x0010),
		}
		gatherCommands = append(gatherCommands, comand)
	} else if dtype == "0007" {
		// 温度8路
		comand := GatherCommand{
			ReadDataLength: 21,
			ModbusAddress:  byte(modbus),
			ModelType:      dtype,
			Writes:         GetCrcCommand(byte(modbus), 0x04, 0x0000, 0x0008),
		}

		gatherCommands = append(gatherCommands, comand)
	} else if dtype == "0008" {
		// 温度6路
		comand := GatherCommand{
			ReadDataLength: 17,
			ModbusAddress:  byte(modbus),
			ModelType:      dtype,
			Writes:         GetCrcCommand(byte(modbus), 0x04, 0x0000, 0x0006),
		}
		fmt.Println("温度6路========================================================")
		fmt.Println(comand.Writes)
		gatherCommands = append(gatherCommands, comand)
	} else if dtype == "0009" {
		// DTSD1352 电流电压
		comand := GatherCommand{
			ReadDataLength: 17,
			ModbusAddress:  byte(modbus),
			ModelType:      dtype,
			Writes:         GetCrcCommand(byte(modbus), 0x03, 0x0061, 0x0006),
		}
		gatherCommands = append(gatherCommands, comand)
		// 总功率因素
		comand = GatherCommand{
			ReadDataLength: 7,
			ModbusAddress:  byte(modbus),
			ModelType:      dtype,
			Writes:         GetCrcCommand(byte(modbus), 0x03, 0x0076, 0x0001),
		}
		gatherCommands = append(gatherCommands, comand)
		// 总有功功率
		comand = GatherCommand{
			ReadDataLength: 9,
			ModbusAddress:  byte(modbus),
			ModelType:      dtype,
			Writes:         GetCrcCommand(byte(modbus), 0x03, 0x006A, 0x0002),
		}
		gatherCommands = append(gatherCommands, comand)
	} else if dtype == "0014" {
		// DTSD1352 电流电压 新型号
		comand := GatherCommand{
			ReadDataLength: 17,
			ModbusAddress:  byte(modbus),
			ModelType:      dtype,
			Writes:         GetCrcCommand(byte(modbus), 0x03, 0x0061, 0x0006),
		}
		gatherCommands = append(gatherCommands, comand)
		// 总功率因素
		comand = GatherCommand{
			ReadDataLength: 7,
			ModbusAddress:  byte(modbus),
			ModelType:      dtype,
			Writes:         GetCrcCommand(byte(modbus), 0x03, 0x017F, 0x0001),
		}
		gatherCommands = append(gatherCommands, comand)
		// 总有功功率
		comand = GatherCommand{
			ReadDataLength: 9,
			ModbusAddress:  byte(modbus),
			ModelType:      dtype,
			Writes:         GetCrcCommand(byte(modbus), 0x03, 0x016A, 0x0002),
		}
		gatherCommands = append(gatherCommands, comand)
	}
	return gatherCommands
}

func GetCrcCommand(address byte, funcode byte, startaddress int16, length int16) []byte {
	var buf []byte
	buf = make([]byte, 8)
	buf[0] = address
	buf[1] = funcode
	buf[2] = byte(startaddress >> 8)
	buf[3] = byte(startaddress)
	buf[4] = byte(length >> 8)
	buf[5] = byte(length)
	var crc crc.Crc
	crc.Reset().PushBytes(buf[0:6])
	checksum := crc.Value()
	buf[7] = byte(checksum >> 8)
	buf[6] = byte(checksum)
	return buf
}

func (client *Client) Run() {
	go client.Write()
	go client.Read()
}
