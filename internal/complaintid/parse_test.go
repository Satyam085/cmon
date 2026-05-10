package complaintid

import "testing"

func TestFromText(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "telegram outgoing format",
			in:   "📋 Complaint : 12345\n\nblah",
			want: "12345",
		},
		{
			name: "whatsapp outgoing format",
			in:   "📋 Complaint: 12345\n\nblah",
			want: "12345",
		},
		{
			name: "header not on first line",
			in:   "Reply preamble\n📋 Complaint: 99999\n\nbody",
			want: "99999",
		},
		{
			name: "leading whitespace on header line",
			in:   "    📋 Complaint : 7\n",
			want: "7",
		},
		{
			name: "trailing whitespace on number",
			in:   "📋 Complaint :   54321   ",
			want: "54321",
		},
		{
			name: "empty text",
			in:   "",
			want: "",
		},
		{
			name: "whitespace only",
			in:   "   \n  ",
			want: "",
		},
		{
			name: "no header line at all",
			in:   "just some random text",
			want: "",
		},
		{
			name: "header without colon is ignored",
			in:   "📋 Complaint 12345",
			want: "",
		},
		{
			name: "header without number is ignored",
			in:   "📋 Complaint: ",
			want: "",
		},
		{
			name: "first valid header wins when multiple present",
			in:   "📋 Complaint: 11111\n📋 Complaint: 22222",
			want: "11111",
		},
		{
			name: "alphanumeric complaint id",
			in:   "📋 Complaint : ABC-123",
			want: "ABC-123",
		},
		{
			name: "different emoji prefix is rejected",
			in:   "🔔 Complaint: 12345",
			want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := FromText(tc.in); got != tc.want {
				t.Errorf("FromText(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
