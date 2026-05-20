package manager

import (
	"errors"
	"reflect"
	"testing"
)

func TestContainsArg(t *testing.T) {
	t.Parallel()

	if !containsArg([]string{"pip", "install", "--prefer-binary"}, "--prefer-binary") {
		t.Fatalf("expected to find --prefer-binary")
	}
	if containsArg([]string{"pip", "install"}, "--prefer-binary") {
		t.Fatalf("did not expect to find --prefer-binary")
	}
}

func TestRemoveFirstArg(t *testing.T) {
	t.Parallel()

	args := []string{"pip", "install", "--prefer-binary", "-r", "requirements.txt", "--prefer-binary"}
	got := removeFirstArg(args, "--prefer-binary")
	want := []string{"pip", "install", "-r", "requirements.txt", "--prefer-binary"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected args after remove, got=%v want=%v", got, want)
	}
}

func TestIsPreferBinaryUnsupportedError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "pip no such option",
			err:  errors.New("install dependencies failed: exit status 2, output: no such option: --prefer-binary"),
			want: true,
		},
		{
			name: "unknown option",
			err:  errors.New("unknown option --prefer-binary"),
			want: true,
		},
		{
			name: "non related error",
			err:  errors.New("connection timeout"),
			want: false,
		},
		{
			name: "nil",
			err:  nil,
			want: false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := isPreferBinaryUnsupportedError(tc.err)
			if got != tc.want {
				t.Fatalf("unexpected result, got=%v want=%v err=%v", got, tc.want, tc.err)
			}
		})
	}
}
