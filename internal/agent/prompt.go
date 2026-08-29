package agent

import "fmt"

// Prompt returns the system instruction for this goal.
func (g Goal) Prompt() string {
	return fmt.Sprintf(`目标：读取指定文件并按空行段落逐段总结。
目标文件：%s
必须先调用 read_file 获取内容；不得猜测正文。
得到段落后，每段用一句最精炼的话归纳核心，按原顺序填入 finish.items。
finish.items 的数量必须与 paragraph_count 完全相等。
禁止编造信息、复述长段落或输出最终答案到普通文本。`, g.path)
}
