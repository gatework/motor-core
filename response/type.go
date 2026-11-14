/*
 * @Author: lwnmengjing
 * @Date: 2021/6/8 5:51 下午
 * @Last Modified by: lwnmengjing
 * @Last Modified time: 2021/6/8 5:51 下午
 */

package response

// Responses defines the fields populated by response helpers.
type Responses interface {
	SetCode(code int32)
	SetTraceID(traceID string)
	SetMsg(message string)
	SetData(data any)
	SetSuccess(success bool)
	Clone() Responses
}
