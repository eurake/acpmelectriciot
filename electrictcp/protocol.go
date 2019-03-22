/******************************************************
# DESC    : 报警判读并且数据存储
# AUTHOR  : Lwdsun
# EMAIL   : lwdsun@sina.com
# MOD     : 2018-06-05
            2018-07-23
# FILE    : protocol.go
******************************************************/
package electrictcp

import (
	"fmt"

	"github.com/eurake/acpm/model"
	"github.com/eurake/xutils"
)

// 协议解析

func ProcessAlarmAndSave(edviceId uint, field string, value interface{}, typefield string) {
	var variable model.EVariable
	err := model.DB.Where("e_device_id = ? AND v_key =?", edviceId, field).First(&variable).Error
	if err != nil {
		return
	}

	if variable.Ratio != 0 {
		tmp := value.(float64)
		tmp = tmp * float64(variable.Ratio)
		value = xutils.Decimal(tmp)
	}

	if field == "sm" {
		fmt.Println("烟感特别处理")
		if value.(float64) > 500.0 {
			value = 1.0
		} else {
			value = 0.0
		}
	}

	//
	if variable.UpUsed {
		if value.(float64) > variable.UpAlarmValue && variable.State == false {
			variable.State = true
			alarm := model.EAlarm{
				EVariableID: variable.ID,
				IsRead:      false,
				AlarmType:   1,
				Context:     "超出设置的报警值报警",
			}
			model.DB.Save(&alarm)
		}
	}
	if variable.DownUsed {
		if value.(float64) < variable.DownAlarmValue && variable.State == false {
			variable.State = true
			alarm := model.EAlarm{
				EVariableID: variable.ID,
				IsRead:      false,
				AlarmType:   2,
				Context:     "低于设置的报警值报警",
			}
			model.DB.Save(&alarm)
		}
	}
	if variable.XorUsed {
		var flag bool
		if value.(float64) == 0.0 {
			flag = false
		} else {
			flag = true
		}
		if flag != variable.XorAlarm && variable.State == false {
			variable.State = true
			alarm := model.EAlarm{
				EVariableID: variable.ID,
				IsRead:      false,
				AlarmType:   3,
				Context:     "开关异常报警",
			}
			model.DB.Save(&alarm)
		}
	}
	//
	fmt.Println("存储")
	fmt.Println(value)

	if variable.Unit == "KW" || variable.Unit == "kw" || variable.Unit == "KV" || variable.Unit == "kv" {
		tmp := value.(float64) / 1000.0
		err = model.DB.Model(&variable).Update("value", tmp, "state", variable.State).Error
	} else {
		err = model.DB.Model(&variable).Update("value", value.(float64), "state", variable.State).Error
	}

	if err != nil {
		fmt.Println("save fail")
		fmt.Println(err)
	}
	if field == "sm" {
		fmt.Println("2-----------------------------------------------------------------------")
	}
}
