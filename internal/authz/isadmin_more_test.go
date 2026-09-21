package authz

import "testing"

// The platform tenant super-roles ("owner" and "admin") are admin-equivalent;
// "operator"/"member" and unknown roles are not. Regression guard for the GUI
// "forbidden" bug where the first operator (provisioned with role "owner") was
// denied collection reads.
func TestIsAdminRecognizesOwnerAndAdmin(t *testing.T) {
	cases := []struct {
		roles []string
		want  bool
	}{
		{[]string{"owner"}, true},
		{[]string{"admin"}, true},
		{[]string{"owner", "operator"}, true}, // first-operator role set
		{[]string{"operator"}, false},
		{[]string{"member"}, false},
		{[]string{"auditor"}, false},
		{nil, false},
	}
	for _, c := range cases {
		if got := (Subjects{Roles: c.roles}).IsAdmin(); got != c.want {
			t.Errorf("IsAdmin(%v) = %v, want %v", c.roles, got, c.want)
		}
	}
}
