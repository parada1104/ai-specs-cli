package gates

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// goldenPlan reads the golden plan the grammar must keep parsing.
func goldenPlan(t *testing.T) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("testdata", "plan_golden.md"))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	return string(content)
}

// planBlockers parses and fails the test when the plan is invalid.
func planBlockers(t *testing.T, text string) Plan {
	t.Helper()
	plan, blockers := ParsePlan(text)
	if len(blockers) != 0 {
		t.Fatalf("valid plan = %+v, want no blockers", blockers)
	}
	return plan
}

func assertBlockers(t *testing.T, blockers []string, needles ...string) {
	t.Helper()
	joined := strings.ToLower(strings.Join(blockers, "\n"))
	for _, needle := range needles {
		if !strings.Contains(joined, strings.ToLower(needle)) {
			t.Fatalf("blockers %q do not contain %q", blockers, needle)
		}
	}
}

// planFixture wraps the given section bodies in a minimal valid plan.
func planFixture(specs, tasks, rest string) string {
	if specs == "" {
		specs = "- **S1.** Fixture requirement\n"
	}
	if tasks == "" {
		tasks = "| ID | Spec | Tarea | Ruta | Commit |\n| --- | --- | --- | --- | --- |\n" +
			"| T1 | S1 | Fixture task | inline | pendiente |\n"
	}
	text := "# fixture\n\n## Specs\n\n" + specs + "\n## Tasks\n\n" + tasks + "\n"
	if rest != "" {
		text += rest
	}
	return text
}

func TestGoldenPlanParsesFully(t *testing.T) {
	plan := planBlockers(t, goldenPlan(t))

	wantSpecs := []PlanSpec{
		{ID: "S1", Text: "El gate exige el plan y no un `tasks.md` libre — *\"el plan es el contrato de entrega\"*"},
		{ID: "S2", Text: "Cada fila de tareas cita el `S#` que implementa — *\"tasks enlazadas a S#\"*"},
		{ID: "S3", Text: "El log guarda la petición del humano verbatim — *\"L1 es la petición original\"*"},
	}
	if len(plan.Specs) != len(wantSpecs) {
		t.Fatalf("specs = %+v, want %d", plan.Specs, len(wantSpecs))
	}
	for index, want := range wantSpecs {
		if plan.Specs[index] != want {
			t.Fatalf("spec %d = %+v, want %+v", index, plan.Specs[index], want)
		}
	}

	wantTasks := []PlanTask{
		{ID: "T1", Specs: "S1", Text: "Parser del plan con un test por regla", Route: "worker", Commit: "`aaaaaaa`"},
		{ID: "T2", Specs: "S1, S2", Text: "Cablear el parser en el gate de cada tier", Route: "worker", Commit: "pendiente"},
		{ID: "T3", Specs: "—", Text: "Cerrar la revisión con el log y el veredicto", Route: "inline", Commit: "pendiente"},
	}
	if len(plan.Tasks) != len(wantTasks) {
		t.Fatalf("tasks = %+v, want %d", plan.Tasks, len(wantTasks))
	}
	for index, want := range wantTasks {
		if plan.Tasks[index] != want {
			t.Fatalf("task %d = %+v, want %+v", index, plan.Tasks[index], want)
		}
	}

	wantLog := []PlanLogEntry{{ID: "L1", Text: "Petición original: *\"implementa I2 con un test por regla\"*"}}
	if len(plan.Log) != 1 || plan.Log[0] != wantLog[0] {
		t.Fatalf("log = %+v, want %+v", plan.Log, wantLog)
	}

	wantVerification := []PlanVerification{
		{ID: "S1", Verdict: "PASS", Evidence: "`go test ./internal/gates/...`, exit 0, commit `aaaaaaa`, 2026-01-01"},
		{ID: "S2", Verdict: "PASS", Evidence: "`go vet ./internal/gates/...`, exit 0, commit `aaaaaaa`, 2026-01-01"},
		{ID: "S3", Verdict: "N/A", Evidence: "la redacción verbatim es prosa y el CLI no la puede juzgar"},
	}
	if len(plan.Verification) != len(wantVerification) {
		t.Fatalf("verification = %+v, want %d rows", plan.Verification, len(wantVerification))
	}
	for index, want := range wantVerification {
		if plan.Verification[index] != want {
			t.Fatalf("verification %d = %+v, want %+v", index, plan.Verification[index], want)
		}
	}
}

func TestPlanAcceptsTaskAsTheThirdHeaderCell(t *testing.T) {
	tasks := "| ID | Spec | Task | Ruta | Commit |\n| --- | --- | --- | --- | --- |\n" +
		"| T1 | S1 | Fixture task | inline | pendiente |\n"
	plan := planBlockers(t, planFixture("", tasks, ""))
	if len(plan.Tasks) != 1 {
		t.Fatalf("tasks = %+v, want one row", plan.Tasks)
	}
}

func TestDuplicatePlanSpecBlocks(t *testing.T) {
	_, blockers := ParsePlan(planFixture("- **S1.** Primero\n- **S1.** Otra vez\n", "", ""))
	assertBlockers(t, blockers, "duplicate id S1")
}

func TestPlanSpecsMustAscendFromS1(t *testing.T) {
	tests := map[string]string{
		"gap":        "- **S1.** Primero\n- **S3.** Tercero\n",
		"notS1":      "- **S2.** Segundo\n- **S3.** Tercero\n",
		"descending": "- **S2.** Segundo\n- **S1.** Primero\n",
	}
	for name, specs := range tests {
		t.Run(name, func(t *testing.T) {
			_, blockers := ParsePlan(planFixture(specs, "", ""))
			assertBlockers(t, blockers, "must ascend from S1")
		})
	}
}

func TestPlanSpecsMustDeclareAtLeastOneBullet(t *testing.T) {
	_, blockers := ParsePlan(planFixture("Solo prosa, ningún bullet.\n", "", ""))
	assertBlockers(t, blockers, "must declare at least one `S#` bullet")
}

func TestTaskRowCitingUndeclaredSpecBlocks(t *testing.T) {
	tasks := "| ID | Spec | Tarea | Ruta | Commit |\n| --- | --- | --- | --- | --- |\n" +
		"| T1 | S9 | Fixture task | inline | pendiente |\n"
	_, blockers := ParsePlan(planFixture("", tasks, ""))
	assertBlockers(t, blockers, "row T1", "undeclared spec S9")
}

func TestTaskRowWithoutSpecBlocks(t *testing.T) {
	tasks := "| ID | Spec | Tarea | Ruta | Commit |\n| --- | --- | --- | --- | --- |\n" +
		"| T1 |  | Fixture task | inline | pendiente |\n"
	_, blockers := ParsePlan(planFixture("", tasks, ""))
	assertBlockers(t, blockers, "empty Spec cell")
}

func TestPlanTasksHeaderOrderIsNormative(t *testing.T) {
	tests := map[string]string{
		"swapped": "| ID | Tarea | Spec | Ruta | Commit |\n| --- | --- | --- | --- | --- |\n" +
			"| T1 | Fixture task | S1 | inline | pendiente |\n",
		"missing": "| ID | Spec | Tarea | Ruta |\n| --- | --- | --- | --- |\n" +
			"| T1 | S1 | Fixture task | inline |\n",
		"renamed": "| ID | Spec | Trabajo | Ruta | Commit |\n| --- | --- | --- | --- | --- |\n" +
			"| T1 | S1 | Fixture task | inline | pendiente |\n",
	}
	for name, tasks := range tests {
		t.Run(name, func(t *testing.T) {
			_, blockers := ParsePlan(planFixture("", tasks, ""))
			assertBlockers(t, blockers, "header must be | ID | Spec | Tarea | Ruta | Commit |")
		})
	}
}

func TestPlanRequiresSpecsAndTasksSections(t *testing.T) {
	t.Run("missing specs", func(t *testing.T) {
		_, blockers := ParsePlan("## Tasks\n\n" +
			"| ID | Spec | Tarea | Ruta | Commit |\n| --- | --- | --- | --- | --- |\n" +
			"| T1 | S1 | Fixture task | inline | pendiente |\n")
		assertBlockers(t, blockers, "`## Specs` section is missing")
	})
	t.Run("missing tasks", func(t *testing.T) {
		_, blockers := ParsePlan("## Specs\n\n- **S1.** Primero\n")
		assertBlockers(t, blockers, "`## Tasks` section is missing")
	})
}

func TestDuplicatePlanSectionBlocks(t *testing.T) {
	text := planFixture("", "", "") + "\n## Specs\n\n- **S2.** Repetida\n"
	_, blockers := ParsePlan(text)
	assertBlockers(t, blockers, "`## Specs` appears more than once")
}

func TestVerificationNARequiresAReason(t *testing.T) {
	_, blockers := ParsePlan(planFixture("", "", "## Verification\n\n- **S1: N/A**\n"))
	assertBlockers(t, blockers, "row S1 is N/A without a reason")

	plan := planBlockers(t, planFixture("", "", "## Verification\n\n- **S1: N/A** — la prosa no se prueba\n"))
	if len(plan.Verification) != 1 || plan.Verification[0].Evidence != "la prosa no se prueba" {
		t.Fatalf("verification = %+v, want the N/A reason", plan.Verification)
	}
}

func TestPlanLogRequiresL1(t *testing.T) {
	_, blockers := ParsePlan(planFixture("", "", "## Log\n\n- **L2** — segunda corrección\n"))
	assertBlockers(t, blockers, "`## Log` must start at L1")

	plan := planBlockers(t, planFixture("", "", "## Log\n\n- **L1** — petición original\n- **L2** — corrección\n"))
	if len(plan.Log) != 2 || plan.Log[0].Text != "petición original" {
		t.Fatalf("log = %+v, want both entries", plan.Log)
	}
}

func TestPlanIgnoresUnknownSectionsAndProse(t *testing.T) {
	plan := planBlockers(t, planFixture("", "", "## Design\n\nProsa sin gramática.\n\n## Notes\n\nMás prosa.\n"))
	if len(plan.Specs) != 1 || len(plan.Tasks) != 1 {
		t.Fatalf("plan = %+v, want the known sections only", plan)
	}
}
