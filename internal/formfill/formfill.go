// Package formfill generates a self-contained browser console snippet that
// fills standard job-application forms (Greenhouse, Lever, and a generic
// label-matching fallback) from profile data.
//
// Design constraints, in order:
//  1. NEVER submit. The snippet touches no buttons and no form.submit();
//     it fills fields, highlights them, and tells the human what to review.
//  2. Zero dependencies. No CDP, no driver binary — the human pastes the
//     snippet into DevTools on the application page they already have open
//     in their logged-in browser. Works everywhere a console works.
//  3. React-safe. Values are set through the native value setter and
//     followed by input/change events so controlled forms register them.
package formfill

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Data is everything the snippet may fill.
type Data struct {
	FullName  string `json:"full_name"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
	Phone     string `json:"phone"`
	Location  string `json:"location"`
	LinkedIn  string `json:"linkedin,omitempty"`
	GitHub    string `json:"github,omitempty"`
	Website   string `json:"website,omitempty"`
	Company   string `json:"current_company,omitempty"` // most recent employer
}

// SplitName fills FirstName/LastName from FullName when unset.
func (d *Data) SplitName() {
	if d.FirstName != "" || d.FullName == "" {
		return
	}
	fields := strings.Fields(d.FullName)
	if len(fields) == 1 {
		d.FirstName = fields[0]
		return
	}
	d.FirstName = strings.Join(fields[:len(fields)-1], " ")
	d.LastName = fields[len(fields)-1]
}

// PickLinks assigns LinkedIn/GitHub/Website from labeled profile links.
func (d *Data) PickLinks(links map[string]string) {
	for label, url := range links {
		l := strings.ToLower(label)
		switch {
		case strings.Contains(l, "linkedin"):
			d.LinkedIn = url
		case strings.Contains(l, "github"):
			d.GitHub = url
		case strings.Contains(l, "portfolio"), strings.Contains(l, "website"), strings.Contains(l, "site"):
			d.Website = url
		}
	}
}

// Snippet renders the console script. It is deterministic for a given Data.
func Snippet(d Data) (string, error) {
	d.SplitName()
	payload, err := json.Marshal(d)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(template, payload), nil
}

// The %s slot receives the JSON payload. The script:
//   - fills by provider-specific selectors first (Greenhouse, Lever),
//   - then generic label matching for anything still empty,
//   - highlights every field it set and reports a fill summary,
//   - leaves file uploads, free-text questions, EEO questions, and the
//     submit button strictly alone.
const template = `(() => {
  // jobkit form fill — REVIEW EVERYTHING BEFORE SUBMITTING. This script
  // never submits; attach the correct tailored resume yourself.
  const D = %s;
  const filled = [];
  const setVal = (el, val) => {
    if (!el || !val || el.type === 'file' || el.disabled || el.readOnly) return false;
    const proto = el.tagName === 'TEXTAREA' ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
    const setter = Object.getOwnPropertyDescriptor(proto, 'value');
    if (setter && setter.set) { setter.set.call(el, val); } else { el.value = val; }
    el.dispatchEvent(new Event('input', { bubbles: true }));
    el.dispatchEvent(new Event('change', { bubbles: true }));
    el.style.outline = '2px solid #34a853';
    filled.push((el.name || el.id || el.placeholder || 'field') + ' = ' + val);
    return true;
  };
  const q = (sel) => document.querySelector(sel);
  const fillFirst = (sels, val) => sels.some((sel) => setVal(q(sel), val));

  // Greenhouse (boards.greenhouse.io / job-boards.greenhouse.io embeds).
  fillFirst(['#first_name', 'input[name="job_application[first_name]"]', 'input[autocomplete="given-name"]'], D.first_name);
  fillFirst(['#last_name', 'input[name="job_application[last_name]"]', 'input[autocomplete="family-name"]'], D.last_name);
  fillFirst(['#email', 'input[name="job_application[email]"]', 'input[type="email"]'], D.email);
  fillFirst(['#phone', 'input[name="job_application[phone]"]', 'input[type="tel"]'], D.phone);
  fillFirst(['#candidate-location', 'input[name="job_application[location]"]', 'input[autocomplete="address-level2"]'], D.location);

  // Lever (jobs.lever.co).
  fillFirst(['input[name="name"]'], D.full_name);
  fillFirst(['input[name="org"]'], D.current_company);
  fillFirst(['input[name="urls[LinkedIn]"]'], D.linkedin);
  fillFirst(['input[name="urls[GitHub]"]'], D.github);
  fillFirst(['input[name="urls[Portfolio]"]', 'input[name="urls[Other]"]'], D.website);

  // Generic fallback: match visible labels to still-empty inputs.
  const wants = [
    [/first\s*name/i, D.first_name], [/last\s*name|surname/i, D.last_name],
    [/full\s*name|your\s*name/i, D.full_name], [/e-?mail/i, D.email],
    [/phone|mobile/i, D.phone], [/location|city/i, D.location],
    [/linked\s*in/i, D.linkedin], [/git\s*hub/i, D.github],
    [/portfolio|website|personal\s*site/i, D.website],
    [/current\s*(company|employer)/i, D.current_company],
  ];
  document.querySelectorAll('label').forEach((label) => {
    const input = label.control ||
      (label.htmlFor && document.getElementById(label.htmlFor)) ||
      label.querySelector('input, textarea');
    if (!input || input.value) return;
    for (const [re, val] of wants) {
      if (re.test(label.textContent || '')) { setVal(input, val); break; }
    }
  });

  console.log('%%cjobkit form fill — filled ' + filled.length + ' field(s):', 'font-weight:bold');
  filled.forEach((f) => console.log('  ' + f));
  console.log('%%cNOT filled (yours to do): resume upload, cover letter, custom questions, EEO. Review every field, then submit yourself.', 'color:#ea8600;font-weight:bold');
})();
`
