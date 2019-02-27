package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/yedamao/encoding"

	"github.com/yedamao/go_cmpp/cmpp"
	"github.com/yedamao/go_cmpp/cmpp/protocol"

	"golang.org/x/net/context"
	"golang.org/x/time/rate"
)

var (
	addr         = flag.String("addr", ":7890", "smgw addr(运营商地址)")
	sourceAddr   = flag.String("sourceAddr", "", "源地址，即SP的企业代码")
	sharedSecret = flag.String("secret", "", "登陆密码")

	serviceId = flag.String("serviceId", "", "业务类型，是数字、字母和符号的组合")

	srcId      = flag.String("srcId", "", "SP的接入号码")
	destNumber = flag.String("destNumber", "", "接收手机号码, 86..., 多个使用，分割")
	msg        = flag.String("msg", "", "短信内容")
	loop       = flag.Int("loop", 1, "循环执行的次数")
	speed      = flag.Int("speed", 1, "速率，speed条/秒")
)

func init() {
	flag.Parse()
}

var sequenceID uint32 = 0
var wg sync.WaitGroup

func newSeqNum() uint32 {
	sequenceID++

	return sequenceID
}

func main() {
	wg.Add(2)

	if "" == *sourceAddr || "" == *sharedSecret {
		fmt.Println("Arg error: sourceAddr or sharedSecret must not be empty .")
		flag.Usage()
		os.Exit(-1)
	}

	destNumbers := strings.Split(*destNumber, ",")
	fmt.Println("destNumbers: ", destNumbers)

	ts, err := cmpp.NewCmpp(*addr, *sourceAddr, *sharedSecret, newSeqNum)
	if err != nil {
		fmt.Println("Connection Err", err)
		os.Exit(-1)
	}
	fmt.Println("connect succ")
	// encoding msg
	content := encoding.UTF82GBK([]byte(*msg))

	if len(content) > 140 {
		fmt.Println("msg Err: not suport long sms")
	}

	go receiver(ts)
	go send(ts, destNumbers, content, *loop, *speed)

	wg.Wait()
}

func send(ts *cmpp.Cmpp, destNumbers []string, content []byte, loop int, speed int) {
	defer wg.Done()
	l := rate.NewLimiter(rate.Limit(speed), 1)
	c, _ := context.WithCancel(context.TODO())

	for i := 1; i <= loop; i++ {
		l.Wait(c)
		_, err := ts.Submit(
			1, 1, 1, 0, *serviceId, 0, "", protocol.GB18030,
			"02", "", *srcId, destNumbers, content,
		)
		currentTime := time.Now().Format("2006-01-02 15:04:05")
		if err != nil {
			fmt.Println(currentTime, " Submit ", i, " err ", err)
			// os.Exit(-1)
			return
		}
		fmt.Println(currentTime, " Submit ", i, " Ok")

		// for {
		// 	op, err := ts.Read() // This is blocking
		// 	if err != nil {
		// 		fmt.Println(currentTime, " Read Err:", err)
		// 		break
		// 	}

		// 	switch op.GetHeader().Command_Id {
		// 	case protocol.CMPP_SUBMIT_RESP:
		// 		ts.Terminate()
		// 		if err := op.Ok(); err != nil {
		// 			fmt.Println(err)
		// 		} else {
		// 			fmt.Println(currentTime, " Submit Ok")
		// 		}
		// 		break

		// 	case protocol.CMPP_TERMINATE_RESP:
		// 		fmt.Println(currentTime, " Terminate response")
		// 		ts.Close()
		// 		return

		// 	default:
		// 		fmt.Printf("%s Unexpect CmdId: %0x\n", currentTime, op.GetHeader().Command_Id)
		// 		ts.Close()
		// 		return
		// 	}
		// }
	}
}

func receiver(ts *cmpp.Cmpp) {
	defer wg.Done()
	for {
		op, err := ts.Read() // This is blocking
		currentTime := time.Now().Format("2006-01-02 15:04:05")
		if err != nil {
			fmt.Println(currentTime, " Read Err:", err)
			return
		}

		switch op.GetHeader().Command_Id {
		case protocol.CMPP_SUBMIT_RESP:
			if err := op.Ok(); err != nil {
				fmt.Println(err)
			} else {
				fmt.Println(currentTime, " Submit Ok")
			}

		case protocol.CMPP_DELIVER:
			dlv, ok := op.(*protocol.Deliver)
			if !ok {
				fmt.Println(currentTime, " Type assert error: ", op)
			}

			if dlv.RegisteredDelivery == protocol.IS_REPORT {
				// 状态报告
				rpt, err := protocol.ParseReport(dlv.MsgContent)
				if err != nil {
					fmt.Println(currentTime, " err:", err)
				}
				fmt.Println(rpt)
			} else {
				// 上行短信
				fmt.Println(currentTime, " REPORT:", dlv)
			}
			ts.DeliverResp(dlv.Header.Sequence_Id, dlv.MsgId, protocol.OK)

		case protocol.CMPP_ACTIVE_TEST:
			fmt.Println(currentTime, " recv ActiveTest")
			ts.ActiveTestResp(op.GetHeader().Sequence_Id)

		case protocol.CMPP_TERMINATE:
			fmt.Println(currentTime, " recv Terminate")
			ts.TerminateResp(op.GetHeader().Sequence_Id)
			ts.Close()
			return

		case protocol.CMPP_TERMINATE_RESP:
			fmt.Println(currentTime, " Terminate response")
			ts.Close()
			return

		default:
			fmt.Printf("%s Unexpect CmdId: %0x\n", currentTime, op.GetHeader().Command_Id)
			ts.Close()
			return
		}
	}
}
