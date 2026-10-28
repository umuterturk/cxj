// Package benchfixture writes a deterministic synthetic Java/Spring workspace.
//
// Performance numbers must be reproducible without shipping a real repository, so
// the benchmark fixture is generated rather than checked in. Both the Go engine
// benchmarks and scripts/perf-bench.sh use this one generator, so the two sets of
// numbers describe the same workspace.
//
// Generation is fully deterministic: identical Options always produce byte-identical
// files, so a baseline captured on one machine is comparable on another (modulo CPU).
package benchfixture

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Options size the fixture. The zero value is Default.
type Options struct {
	Modules            int // number of packages to spread types over
	TypesPerModule     int // main types per module (controller+service+repo families)
	TestTypesPerModule int // test classes per module
}

// Default approximates payment-api (561 main / 849 total Java files): 10 modules
// x 14 type families x 4 main files = 560 main files, plus 290 dense test
// classes. Small enough to regenerate inside a CI job, close enough in shape
// that relative wins transfer.
var Default = Options{
	Modules:            10,
	TypesPerModule:     14,
	TestTypesPerModule: 29,
}

func (o Options) withDefaults() Options {
	if o.Modules <= 0 && o.TypesPerModule <= 0 {
		return Default
	}
	if o.Modules <= 0 {
		o.Modules = Default.Modules
	}
	if o.TypesPerModule <= 0 {
		o.TypesPerModule = Default.TypesPerModule
	}
	if o.TestTypesPerModule < 0 {
		o.TestTypesPerModule = 0
	} else if o.TestTypesPerModule == 0 {
		o.TestTypesPerModule = Default.TestTypesPerModule
	}
	return o
}

// Stats reports what was written. Java counts exclude poms and resources so the
// perf baseline can be guarded against the file counts it was calibrated to.
type Stats struct {
	Files         int
	JavaFiles     int
	MainJavaFiles int
	TestJavaFiles int
	Bytes         int
}

// Write materializes the fixture under root and reports what it wrote.
func Write(root string, o Options) (Stats, error) {
	o = o.withDefaults()
	var st Stats
	write := func(rel, body string) error {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			return err
		}
		st.Files++
		st.Bytes += len(body)
		test := strings.HasPrefix(rel, "src/test/") || strings.Contains(rel, "/src/test/")
		if strings.HasSuffix(rel, ".java") {
			st.JavaFiles++
			if test {
				st.TestJavaFiles++
			} else {
				st.MainJavaFiles++
			}
		}
		return nil
	}

	if err := write("pom.xml", pom(o.Modules)); err != nil {
		return st, err
	}
	if err := write("src/main/resources/application.yml", applicationYML()); err != nil {
		return st, err
	}
	if err := write("src/test/resources/application-test.yml", applicationYML()); err != nil {
		return st, err
	}

	for m := 0; m < o.Modules; m++ {
		pkg := "com.example.bench.mod" + strconv.Itoa(m)
		for n := 0; n < o.TypesPerModule; n++ {
			base := "Widget" + strconv.Itoa(m*1000+n)
			if err := write("src/main/java/"+pkgDir(pkg)+"/"+base+"Controller.java", controller(pkg, base, m, n)); err != nil {
				return st, err
			}
			if err := write("src/main/java/"+pkgDir(pkg)+"/"+base+"Service.java", service(pkg, base, m, n)); err != nil {
				return st, err
			}
			if err := write("src/main/java/"+pkgDir(pkg)+"/"+base+"Repository.java", repository(pkg, base)); err != nil {
				return st, err
			}
			if err := write("src/main/java/"+pkgDir(pkg)+"/"+base+"Dto.java", dto(pkg, base)); err != nil {
				return st, err
			}
		}
		for n := 0; n < o.TestTypesPerModule; n++ {
			base := "Gadget" + strconv.Itoa(m*1000+n)
			if err := write("src/test/java/"+pkgDir(pkg)+"/"+base+"Test.java", testClass(pkg, base, m, n)); err != nil {
				return st, err
			}
		}
	}
	return st, nil
}

// EntrySymbol is a stable, resolvable entrypoint in the Default fixture. Benchmarks
// and scripts query it so they always measure a real hit, never a silent miss.
const EntrySymbol = "com.example.bench.mod0.Widget0Controller.get"

func pkgDir(pkg string) string { return strings.ReplaceAll(pkg, ".", "/") }

func pom(modules int) string {
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	b.WriteString("<project xmlns=\"http://maven.apache.org/POM/4.0.0\">\n")
	b.WriteString("  <modelVersion>4.0.0</modelVersion>\n")
	b.WriteString("  <groupId>com.example</groupId>\n")
	b.WriteString("  <artifactId>benchfixture</artifactId>\n")
	b.WriteString("  <version>1.0.0</version>\n")
	b.WriteString("  <parent>\n")
	b.WriteString("    <groupId>org.springframework.boot</groupId>\n")
	b.WriteString("    <artifactId>spring-boot-starter-parent</artifactId>\n")
	b.WriteString("    <version>3.3.5</version>\n")
	b.WriteString("  </parent>\n")
	b.WriteString("  <properties><java.version>21</java.version></properties>\n")
	b.WriteString("  <dependencies>\n")
	b.WriteString("    <dependency><groupId>org.springframework.boot</groupId><artifactId>spring-boot-starter-web</artifactId></dependency>\n")
	b.WriteString("    <dependency><groupId>org.springframework.boot</groupId><artifactId>spring-boot-starter-data-jpa</artifactId></dependency>\n")
	b.WriteString("    <dependency><groupId>com.fasterxml.jackson.core</groupId><artifactId>jackson-databind</artifactId><version>2.17.2</version></dependency>\n")
	b.WriteString("  </dependencies>\n")
	fmt.Fprintf(&b, "  <!-- %d modules -->\n", modules)
	b.WriteString("</project>\n")
	return b.String()
}

func applicationYML() string {
	return "spring:\n" +
		"  application:\n" +
		"    name: benchfixture\n" +
		"  datasource:\n" +
		"    url: jdbc:postgresql://localhost:5432/bench\n" +
		"    username: bench\n" +
		"    password: secret\n" +
		"  jpa:\n" +
		"    hibernate:\n" +
		"      ddl-auto: validate\n" +
		"bench:\n" +
		"  timeout-ms: 2500\n" +
		"  retries: 3\n" +
		"  feature:\n" +
		"    new-router: true\n"
}

// controller is a Spring MVC entrypoint that calls its service and helper, so the
// graph has http routes, injections and intra-type calls.
func controller(pkg, base string, m, n int) string {
	var b strings.Builder
	b.WriteString("package " + pkg + ";\n\n")
	b.WriteString("import java.util.List;\n")
	b.WriteString("import java.util.ArrayList;\n")
	b.WriteString("import org.springframework.web.bind.annotation.GetMapping;\n")
	b.WriteString("import org.springframework.web.bind.annotation.PostMapping;\n")
	b.WriteString("import org.springframework.web.bind.annotation.RequestBody;\n")
	b.WriteString("import org.springframework.web.bind.annotation.PathVariable;\n")
	b.WriteString("import org.springframework.web.bind.annotation.RestController;\n\n")
	b.WriteString("@RestController\n")
	b.WriteString("public class " + base + "Controller {\n\n")
	b.WriteString("    private final " + base + "Service service;\n")
	b.WriteString("    private final " + base + "Repository repository;\n\n")
	b.WriteString("    public " + base + "Controller(" + base + "Service service, " + base + "Repository repository) {\n")
	b.WriteString("        this.service = service;\n")
	b.WriteString("        this.repository = repository;\n")
	b.WriteString("    }\n\n")
	b.WriteString("    @GetMapping(\"/api/v" + strconv.Itoa(m%3) + "/" + strings.ToLower(base) + "/{id}\")\n")
	b.WriteString("    public " + base + "Dto get(@PathVariable Integer id) {\n")
	b.WriteString("        " + base + "Dto found = service.load(id);\n")
	b.WriteString("        return decorate(found);\n")
	b.WriteString("    }\n\n")
	b.WriteString("    @PostMapping(\"/api/v" + strconv.Itoa(m%3) + "/" + strings.ToLower(base) + "\")\n")
	b.WriteString("    public " + base + "Dto create(@RequestBody " + base + "Dto body) {\n")
	b.WriteString("        validate(body);\n")
	b.WriteString("        " + base + "Dto saved = service.save(body);\n")
	b.WriteString("        publish(saved);\n")
	b.WriteString("        return decorate(saved);\n")
	b.WriteString("    }\n\n")
	for i := 0; i < 6; i++ {
		k := strconv.Itoa(i)
		b.WriteString("    private void helper" + k + "(" + base + "Dto dto) {\n")
		b.WriteString("        List<String> acc = new ArrayList<>();\n")
		b.WriteString("        acc.add(dto.getName());\n")
		b.WriteString("        acc.add(dto.getDescription());\n")
		b.WriteString("        service.audit(" + k + ", acc.toString());\n")
		b.WriteString("        repository.touch(" + k + ");\n")
		b.WriteString("    }\n\n")
	}
	b.WriteString("    private " + base + "Dto decorate(" + base + "Dto dto) {\n")
	for i := 0; i < 6; i++ {
		b.WriteString("        helper" + strconv.Itoa(i) + "(dto);\n")
	}
	b.WriteString("        return dto;\n")
	b.WriteString("    }\n\n")
	b.WriteString("    private void validate(" + base + "Dto dto) {\n")
	b.WriteString("        if (dto == null) {\n")
	b.WriteString("            throw new IllegalArgumentException(\"body required\");\n")
	b.WriteString("        }\n")
	b.WriteString("        service.check(dto.getId());\n")
	b.WriteString("    }\n\n")
	b.WriteString("    private void publish(" + base + "Dto dto) {\n")
	b.WriteString("        repository.notifyListeners(dto.getId());\n")
	b.WriteString("    }\n")
	b.WriteString("}\n")
	return b.String()
}

func service(pkg, base string, m, n int) string {
	var b strings.Builder
	b.WriteString("package " + pkg + ";\n\n")
	b.WriteString("import java.util.Optional;\n")
	b.WriteString("import org.springframework.stereotype.Service;\n")
	b.WriteString("import org.springframework.transaction.annotation.Transactional;\n\n")
	b.WriteString("@Service\n")
	b.WriteString("public class " + base + "Service {\n\n")
	b.WriteString("    private final " + base + "Repository repository;\n")
	b.WriteString("    private final " + base + "Mapper mapper = new " + base + "Mapper();\n\n")
	b.WriteString("    public " + base + "Service(" + base + "Repository repository) {\n")
	b.WriteString("        this.repository = repository;\n")
	b.WriteString("    }\n\n")
	b.WriteString("    @Transactional(readOnly = true)\n")
	b.WriteString("    public " + base + "Dto load(Integer id) {\n")
	b.WriteString("        Optional<" + base + "Dto> found = repository.findById(id);\n")
	b.WriteString("        return found.orElseGet(() -> mapper.empty());\n")
	b.WriteString("    }\n\n")
	b.WriteString("    @Transactional\n")
	b.WriteString("    public " + base + "Dto save(" + base + "Dto dto) {\n")
	b.WriteString("        mapper.stamp(dto);\n")
	b.WriteString("        return repository.save(mapper.toEntity(dto));\n")
	b.WriteString("    }\n\n")
	b.WriteString("    public void audit(int attempt, String payload) {\n")
	b.WriteString("        repository.logAttempt(attempt, payload);\n")
	b.WriteString("    }\n\n")
	b.WriteString("    public void check(Integer id) {\n")
	b.WriteString("        if (!repository.exists(id)) {\n")
	b.WriteString("            throw new IllegalStateException(\"missing \" + id);\n")
	b.WriteString("        }\n")
	b.WriteString("    }\n\n")
	b.WriteString("    public int total(" + base + "Query query) {\n")
	b.WriteString("        return repository.countBy(query.getStatus(), query.getFrom(), query.getTo());\n")
	b.WriteString("    }\n\n")
	b.WriteString("    private static class " + base + "Mapper {\n")
	b.WriteString("        " + base + "Dto empty() {\n")
	b.WriteString("            " + base + "Dto dto = new " + base + "Dto();\n")
	b.WriteString("            dto.setName(\"empty\");\n")
	b.WriteString("            return dto;\n")
	b.WriteString("        }\n\n")
	b.WriteString("        void stamp(" + base + "Dto dto) {\n")
	b.WriteString("            dto.setUpdatedAt(System.currentTimeMillis());\n")
	b.WriteString("        }\n\n")
	b.WriteString("        " + base + "Dto toEntity(" + base + "Dto dto) {\n")
	b.WriteString("            return dto;\n")
	b.WriteString("        }\n")
	b.WriteString("    }\n")
	b.WriteString("}\n")
	return b.String()
}

func repository(pkg, base string) string {
	var b strings.Builder
	b.WriteString("package " + pkg + ";\n\n")
	b.WriteString("import java.util.Optional;\n")
	b.WriteString("import org.springframework.stereotype.Repository;\n\n")
	b.WriteString("@Repository\n")
	b.WriteString("public class " + base + "Repository {\n\n")
	b.WriteString("    public Optional<" + base + "Dto> findById(Integer id) {\n")
	b.WriteString("        return Optional.empty();\n")
	b.WriteString("    }\n\n")
	b.WriteString("    public " + base + "Dto save(" + base + "Dto dto) {\n")
	b.WriteString("        return dto;\n")
	b.WriteString("    }\n\n")
	b.WriteString("    public boolean exists(Integer id) { return id != null; }\n\n")
	b.WriteString("    public int countBy(String status, Long from, Long to) { return 0; }\n\n")
	b.WriteString("    public void touch(int slot) { }\n\n")
	b.WriteString("    public void notifyListeners(Integer id) { }\n\n")
	b.WriteString("    public void logAttempt(int attempt, String payload) { }\n")
	b.WriteString("}\n")
	return b.String()
}

func dto(pkg, base string) string {
	var b strings.Builder
	b.WriteString("package " + pkg + ";\n\n")
	b.WriteString("public class " + base + "Dto {\n\n")
	b.WriteString("    private Integer id;\n")
	b.WriteString("    private String name;\n")
	b.WriteString("    private String description;\n")
	b.WriteString("    private String status;\n")
	b.WriteString("    private Long updatedAt;\n\n")
	for _, f := range []string{"id:Integer", "name:String", "description:String", "status:String", "updatedAt:Long"} {
		parts := strings.SplitN(f, ":", 2)
		field, typ := parts[0], parts[1]
		cap := strings.ToUpper(field[:1]) + field[1:]
		b.WriteString("    public " + typ + " get" + cap + "() { return " + field + "; }\n\n")
		b.WriteString("    public void set" + cap + "(" + typ + " " + field + ") { this." + field + " = " + field + "; }\n\n")
	}
	b.WriteString("}\n\n")
	b.WriteString("class " + base + "Query {\n\n")
	b.WriteString("    private String status;\n    private Long from;\n    private Long to;\n\n")
	b.WriteString("    public String getStatus() { return status; }\n")
	b.WriteString("    public Long getFrom() { return from; }\n")
	b.WriteString("    public Long getTo() { return to; }\n")
	b.WriteString("}\n")
	return b.String()
}

// testClass is deliberately invocation-dense, the way real Spring integration
// tests are: many assertions and calls per line. This is what makes skipping
// src/test worth so much more than its file share.
func testClass(pkg, base string, m, n int) string {
	var b strings.Builder
	b.WriteString("package " + pkg + ";\n\n")
	b.WriteString("import java.util.List;\n")
	b.WriteString("import java.util.ArrayList;\n")
	b.WriteString("import org.junit.jupiter.api.Test;\n")
	b.WriteString("import org.springframework.beans.factory.annotation.Autowired;\n")
	b.WriteString("import org.springframework.boot.test.context.SpringBootTest;\n\n")
	b.WriteString("@SpringBootTest\n")
	b.WriteString("public class " + base + "Test {\n\n")
	b.WriteString("    @Autowired\n")
	b.WriteString("    private " + base + "Service service;\n\n")
	b.WriteString("    @Autowired\n")
	b.WriteString("    private " + base + "Repository repository;\n\n")
	for i := 0; i < 8; i++ {
		k := strconv.Itoa(i)
		b.WriteString("    @Test\n")
		b.WriteString("    void scenario" + k + "() {\n")
		b.WriteString("        List<" + base + "Dto> acc = new ArrayList<>();\n")
		b.WriteString("        for (int i = 0; i < 10; i++) {\n")
		b.WriteString("            acc.add(service.load(i));\n")
		b.WriteString("            service.audit(" + k + ", \"scenario" + k + "\");\n")
		b.WriteString("            repository.touch(i);\n")
		b.WriteString("        }\n")
		b.WriteString("        service.check(" + k + ");\n")
		b.WriteString("        repository.exists(" + k + ");\n")
		b.WriteString("        service.save(acc.get(0));\n")
		b.WriteString("        repository.logAttempt(" + k + ", String.valueOf(acc.size()));\n")
		b.WriteString("    }\n\n")
	}
	b.WriteString("}\n")
	return b.String()
}
