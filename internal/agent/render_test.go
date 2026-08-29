package agent

import "testing"

func TestRender(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		result    Result
		truncated []int
		want      string
	}{
		{
			name:   "empty result",
			path:   "/tmp/empty.txt",
			result: Result{Items: []string{}},
			want:   "文件: /tmp/empty.txt\n段数: 0\n\n无有效段落\n",
		},
		{
			name:   "numbered items",
			path:   "/tmp/input.txt",
			result: Result{Items: []string{"第一段", "第二段"}},
			want: "文件: /tmp/input.txt\n段数: 2\n\n" +
				"1. 第一段\n" +
				"2. 第二段\n",
		},
		{
			name:      "truncated paragraphs",
			path:      "/tmp/input.txt",
			result:    Result{Items: []string{"第一段", "第二段", "第三段"}},
			truncated: []int{1, 3},
			want: "文件: /tmp/input.txt\n段数: 3\n\n" +
				"1. [截断] 第一段\n" +
				"2. 第二段\n" +
				"3. [截断] 第三段\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Render(tt.path, tt.result, tt.truncated); got != tt.want {
				t.Fatalf("Render() = %q, want %q", got, tt.want)
			}
		})
	}
}
