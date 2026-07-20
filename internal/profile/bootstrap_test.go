package profile

import "testing"

func TestFromResumeTextBootstrapsProfile(t *testing.T) {
	raw := `SAM RIVERA
(555) 010-2040 | sam.rivera@example.com
github.com/samrivera | linkedin.com/in/samrivera

OBJECTIVE
Software engineer building production TypeScript, React, Go, PostgreSQL and AWS systems.

EXPERIENCE
Vandelay Industries January 2021 - Present
Fullstack Software Engineer
Designed and deployed production React and TypeScript applications.
Built Go and PostgreSQL services on AWS.

SOFTWARE PROJECTS
IdleTime (https://idletime.app)
- Built real-time scheduling with React, Node.js, Docker, MongoDB and Socket.io.

EDUCATION
State University, Austin, TX January 2020 - Dec 2021
Bachelor of Science in Computer Science`

	p := FromResumeText(raw)
	if p.Name != "Sam Rivera" {
		t.Fatalf("name = %q", p.Name)
	}
	if p.Email != "sam.rivera@example.com" {
		t.Fatalf("email = %q", p.Email)
	}
	if len(p.Experience) != 1 || p.Experience[0].Company != "Vandelay Industries" || p.Experience[0].Start != "2021-01" {
		t.Fatalf("experience = %#v", p.Experience)
	}
	if len(p.Skills) == 0 {
		t.Fatal("expected inferred skills")
	}
	if errs := p.Validate(); len(errs) > 0 {
		t.Fatalf("bootstrap profile did not validate: %v", errs)
	}
}
