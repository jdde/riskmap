#!/usr/bin/env python3
"""Generates the .patch files of the acceptance tests from the example projects, using git.

Usage: python3 tests/make-patches.py

Each example project is copied into a temporary git repository. For every case the
edits below are applied and the staged diff is written to
tests/acceptance/<example>/<case>.patch. The script also sets the `patch:` line of
each case in tests/acceptance/<example>.yml. See tests/README.md.
"""
import os, re, shutil, subprocess, tempfile

# This script lives in tests/, so all paths are relative to its own folder.
TESTS = os.path.dirname(os.path.abspath(__file__))
EX = os.path.join(TESTS, "examples")
ACC = os.path.join(TESTS, "acceptance")

def git(cwd, *args, binary=False):
    r = subprocess.run(["git", "-c", "user.name=riskmap", "-c", "user.email=riskmap@example.invalid",
                        "-c", "core.quotePath=true", *args], cwd=cwd, capture_output=True, check=True)
    return r.stdout if binary else r.stdout.decode()

class Repo:
    def __init__(self, d): self.d = d
    def p(self, rel): return os.path.join(self.d, rel)
    def replace(self, rel, old, new):
        s = open(self.p(rel), encoding="utf-8").read()
        assert old in s, (rel, old)
        open(self.p(rel), "w", encoding="utf-8").write(s.replace(old, new, 1))
    def append(self, rel, text):
        with open(self.p(rel), "a", encoding="utf-8") as f: f.write(text)
    def new(self, rel, text):
        os.makedirs(os.path.dirname(self.p(rel)), exist_ok=True)
        assert not os.path.exists(self.p(rel)), rel
        open(self.p(rel), "w", encoding="utf-8").write(text)
    def write_bytes(self, rel, data): open(self.p(rel), "wb").write(data)
    def delete(self, rel): os.remove(self.p(rel))
    def move(self, a, b):
        os.makedirs(os.path.dirname(self.p(b)), exist_ok=True)
        git(self.d, "mv", a, b)

LOWER_DEFAULT = ("default: { risk: high }", "default: { risk: low }")

def nextjs(r):
    def docs_only():
        r.append("README.md", "\n## Development\n\n`npm run dev` starts the development server.\n")
        r.append("docs/architecture.md", "- `public`: static files\n")
    def docs_special():
        # The file name is deliberate: a space and a non-ASCII character test the path handling.
        r.new("docs/Über uns.md", "# About us\n\nA small example shop.\n")
    def component_with_test():
        r.replace("src/components/Button.tsx", '  type?: "button" | "submit";\n', '  type?: "button" | "submit";\n  disabled?: boolean;\n')
        r.replace("src/components/Button.tsx", 'type = "button" }: ButtonProps', 'type = "button", disabled = false }: ButtonProps')
        r.replace("src/components/Button.tsx", "<button type={type}>", "<button type={type} disabled={disabled}>")
        r.replace("src/components/Button.test.tsx", "});\n});\n",
                  '});\n\n  it("can be disabled", () => {\n    expect(renderToString(<Button disabled>Go</Button>)).toContain("disabled");\n  });\n});\n')
    def homepage():
        r.replace("src/app/page.tsx", "<h1>Shop</h1>", '<h1 className="title">Shop</h1>')
        r.append("src/app/globals.css", "\n.title {\n  font-size: 2rem;\n}\n")
    def payment_test():
        r.replace("src/lib/stripe.test.ts", "  });\n});\n",
                  '  });\n\n  it("provides webhooks", () => {\n    expect(stripe.webhooks).toBeDefined();\n  });\n});\n')
    def checkout_page():
        r.replace("src/app/(shop)/checkout/page.tsx", "Pay now", "Order and pay")
    def feature():
        r.replace("src/app/page.tsx", "<h1>Shop</h1>", "<h1>Shop</h1>\n      <p>Ships in two days.</p>")
        r.replace("src/lib/stripe.ts", "process.env.STRIPE_SECRET_KEY!", 'process.env.STRIPE_SECRET_KEY!, {\n  maxNetworkRetries: 2,\n}')
        r.replace("prisma/schema.prisma", "  paid      Boolean @default(false)\n", "  paid      Boolean @default(false)\n  shippedAt DateTime?\n")
        r.append("docs/architecture.md", "- Orders remember when they were shipped\n")
    def unmatched():
        r.append(".env.example", 'SHIPPING_API_KEY=""\n')
    def near_miss():
        r.new("docs2/readme.md", "# Notes\n")
        r.new("src/components2/Button.tsx", "export function Button() {\n  return <button />;\n}\n")
        r.new("apps/web/package.json", '{\n  "name": "web",\n  "private": true\n}\n')
    def rename():
        r.move("src/lib/auth.ts", "docs/auth.md")
    def ci():
        r.replace(".github/workflows/ci.yml", "      - run: npm run lint\n", "")
    def config():
        r.replace(".riskmap.yml", *LOWER_DEFAULT)
    def config_docs():
        config(); r.append("README.md", "\nStatus: example.\n")
    return {"docs-only": docs_only, "docs-with-special-characters": docs_special,
            "component-with-test": component_with_test, "homepage": homepage,
            "payment-test-only": payment_test, "checkout-page": checkout_page,
            "feature-across-layers": feature, "unmatched-file": unmatched, "near-miss": near_miss,
            "rename-out-of-auth": rename, "ci-workflow": ci, "riskmap-config": config,
            "riskmap-config-with-docs": config_docs}

def laravel(r):
    def docs_only():
        r.append("README.md", "\n## Tests\n\n`php artisan test`\n")
        r.append("docs/api.md", "| GET | `/api/orders/{id}` | a single order |\n")
    def tests_only():
        r.replace("tests/Feature/OrderTest.php", "            ->assertCreated();\n    }\n",
                  "            ->assertCreated();\n    }\n\n    public function test_a_guest_cannot_list_orders(): void\n    {\n        $this->getJson('/api/orders')->assertUnauthorized();\n    }\n")
        r.replace("database/factories/UserFactory.php", "            'password' => Hash::make('password'),\n", "            'password' => Hash::make('password'),\n            'tenant_id' => 1,\n")
    def deleted_test():
        r.delete("tests/Unit/InvoiceServiceTest.php")
    def controller_with_test():
        r.replace("app/Http/Controllers/OrderController.php", "->latest()->get();", "->latest()->limit(50)->get();")
        r.replace("tests/Feature/OrderTest.php", "['total_cents' => 1000])\n", "['total_cents' => 2500])\n")
    def view_seeder():
        r.replace("resources/views/welcome.blade.php", "        <h1>{{ config('app.name') }}</h1>\n",
                  "        <h1>{{ config('app.name') }}</h1>\n        <p>API ready.</p>\n")
        r.replace("database/seeders/DatabaseSeeder.php", "['email' => 'demo@example.com']", "['email' => 'demo@example.com', 'tenant_id' => 1]")
    def billing():
        r.replace("app/Services/Billing/InvoiceService.php", "* 0.19)", "* 0.07)")
    def order_model():
        r.replace("app/Models/Order.php", "['total_cents']", "['total_cents', 'status']")
    def models():
        order_model()
        r.replace("app/Models/User.php", "'tenant_id'];", "'tenant_id', 'phone'];")
    def new_migration():
        r.new("database/migrations/2026_03_01_000000_add_status_to_orders_table.php",
              "<?php\n\nuse Illuminate\\Database\\Migrations\\Migration;\nuse Illuminate\\Database\\Schema\\Blueprint;\nuse Illuminate\\Support\\Facades\\Schema;\n\nreturn new class extends Migration\n{\n    public function up(): void\n    {\n        Schema::table('orders', function (Blueprint $table) {\n            $table->string('status')->default('open');\n        });\n    }\n\n    public function down(): void\n    {\n        Schema::table('orders', function (Blueprint $table) {\n            $table->dropColumn('status');\n        });\n    }\n};\n")
    def feature():
        r.replace("app/Http/Controllers/OrderController.php", "        return $order;\n    }\n",
                  "        return $order;\n    }\n\n    public function destroy(Order $order)\n    {\n        $this->authorize('view', $order);\n        $order->delete();\n\n        return response()->noContent();\n    }\n")
        r.replace("routes/api.php", "['index', 'store', 'show']", "['index', 'store', 'show', 'destroy']")
        r.replace("database/migrations/2026_01_15_000000_create_orders_table.php", "            $table->timestamps();\n", "            $table->timestamps();\n            $table->softDeletes();\n")
        r.replace("tests/Feature/OrderTest.php", "            ->assertCreated();\n    }\n",
                  "            ->assertCreated();\n    }\n\n    public function test_a_user_can_delete_an_order(): void\n    {\n        $user = User::factory()->create(['tenant_id' => 1]);\n        $order = $user->orders()->create(['total_cents' => 1000]);\n\n        $this->actingAs($user)->deleteJson(\"/api/orders/{$order->id}\")->assertNoContent();\n    }\n")
    def unmatched():
        r.replace("phpunit.xml", '        <env name="APP_ENV" value="testing"/>\n', '        <env name="APP_ENV" value="testing"/>\n        <env name="CACHE_STORE" value="array"/>\n')
    def near_miss():
        r.new("application/Models/User.php", "<?php\n\nnamespace Application\\Models;\n\nclass User\n{\n}\n")
        r.new("tests2/OrderTest.php", "<?php\n\nclass OrderTest\n{\n}\n")
        r.new("packages/billing/composer.json", '{\n    "name": "example/billing"\n}\n')
    def rename():
        r.move("app/Policies/OrderPolicy.php", "docs/policy.md")
    def config():
        r.replace(".riskmap.yml", *LOWER_DEFAULT)
    return {"docs-only": docs_only, "tests-only": tests_only, "deleted-test": deleted_test,
            "controller-with-test": controller_with_test, "view-and-seeder": view_seeder, "broad-and-narrow-rule": billing,
            "order-model": order_model, "models-together": models, "new-migration": new_migration,
            "feature-with-migration": feature, "unmatched-file": unmatched, "near-miss": near_miss,
            "rename-policy-to-docs": rename, "riskmap-config": config}

PNG2 = bytes.fromhex("89504e470d0a1a0a0000000d49484452000000010000000108060000001f15c489"
                     "0000000d49444154789c6360606060000000050001a5f645400000000049454e44ae426082")

def flutter(r):
    def translations():
        r.replace("lib/l10n/app_de.arb", '  "welcome": "Willkommen"\n', '  "welcome": "Willkommen",\n  "signOut": "Abmelden"\n')
        r.replace("lib/l10n/app_en.arb", '  "welcome": "Welcome"\n', '  "welcome": "Welcome",\n  "signOut": "Sign out"\n')
    def assets_docs():
        r.write_bytes("assets/images/logo.png", PNG2)
        r.append("README.md", "\nThe logo is in `assets/images/`.\n")
    def screen_with_test():
        r.replace("lib/features/home/home_screen.dart", "Text('Welcome')", "Text('Welcome back')")
        r.replace("lib/widgets/primary_button.dart", "child: Text(label));", "child: Text(label.toUpperCase()));")
        r.replace("test/widget_test.dart", "find.text('Go')", "find.text('GO')")
    def app_shell():
        r.replace("lib/app.dart", "title: 'Flutter App',", "title: 'Flutter App',\n      debugShowCheckedModeBanner: false,")
    def auth_service():
        r.replace("lib/features/login/auth_service.dart", "  void signOut() => _token = null;\n",
                  "  void signOut() => _token = null;\n\n  Map<String, String> get headers => {'Authorization': 'Bearer $_token'};\n")
    def auth_test():
        r.replace("test/features/login/auth_service_test.dart", "  });\n}\n",
                  "  });\n\n  test('is signed out at first', () {\n    expect(AuthService().isSignedIn, isFalse);\n  });\n}\n")
    def android():
        r.replace("android/app/src/main/AndroidManifest.xml", '    <uses-permission android:name="com.android.vending.BILLING" />\n',
                  '    <uses-permission android:name="com.android.vending.BILLING" />\n    <uses-permission android:name="android.permission.CAMERA" />\n')
    def both():
        r.replace("android/app/build.gradle.kts", "minSdk = flutter.minSdkVersion", "minSdk = 24")
        r.replace("ios/Runner/Info.plist", "\t<key>CFBundleVersion</key>", "\t<key>NSCameraUsageDescription</key>\n\t<string>Photos for the profile</string>\n\t<key>CFBundleVersion</key>")
    def purchase():
        r.replace("lib/features/payments/purchase_service.dart", "{'premium'}", "{'premium', 'premium_yearly'}")
        android()
        r.replace("pubspec.yaml", "  intl: any\n", "  intl: any\n  shared_preferences: ^2.3.0\n")
        r.replace("pubspec.lock", "packages: {}\n", 'packages:\n  shared_preferences:\n    dependency: "direct main"\n    version: "2.3.0"\n')
    def unmatched():
        r.replace("analysis_options.yaml", "    prefer_const_constructors: true\n", "    prefer_const_constructors: false\n")
    def near_miss():
        r.new("library/main.dart", "void main() {}\n")
        r.new("tests/widget_test.dart", "void main() {}\n")
        r.new("packages/core/pubspec.yaml", "name: core\n")
    def rename():
        r.move("lib/features/login/auth_service.dart", "test/auth_service.dart")
    def config():
        r.replace(".riskmap.yml", *LOWER_DEFAULT)
    return {"translations-only": translations, "assets-and-docs": assets_docs, "screen-with-test": screen_with_test,
            "app-shell": app_shell,
            "auth-service": auth_service, "auth-test-only": auth_test, "android-only": android,
            "both-platforms": both, "purchase-feature": purchase, "unmatched-file": unmatched,
            "near-miss": near_miss, "rename-auth-to-test": rename, "riskmap-config": config}

for name, builder in [("nextjs-web", nextjs), ("laravel-api", laravel), ("flutter-app", flutter)]:
    tmp = tempfile.mkdtemp(prefix="riskmap-" + name + "-")
    work = os.path.join(tmp, "p")
    shutil.copytree(os.path.join(EX, name), work)
    git(work, "init", "-q")
    git(work, "add", "-A")
    git(work, "commit", "-q", "-m", "base")
    out = os.path.join(ACC, name)
    shutil.rmtree(out, ignore_errors=True)
    os.makedirs(out)
    cases = builder(Repo(work))
    for cid, fn in cases.items():
        fn()
        git(work, "add", "-A")
        patch = git(work, "diff", "--cached", "-M", "--binary", "--no-color", binary=True)
        assert patch, cid
        open(os.path.join(out, cid + ".patch"), "wb").write(patch)
        git(work, "reset", "-q", "--hard", "HEAD")
        git(work, "clean", "-qfd")
    # Set the patch: line of each case in the acceptance file.
    yml = os.path.join(ACC, name + ".yml")
    s = open(yml, encoding="utf-8").read()
    s = re.sub(r"\n    patch: .*", "", s)
    for cid in cases:
        marker = "  - id: %s\n" % cid
        assert marker in s, (name, cid)
        s = s.replace(marker, marker + '    patch: "%s/%s.patch"\n' % (name, cid), 1)
    open(yml, "w", encoding="utf-8").write(s)
    shutil.rmtree(tmp)
    print(name, len(cases), "patches")
