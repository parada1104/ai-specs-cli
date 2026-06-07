import importlib.util
import sys
import tomllib
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
TOML_WRITE_PATH = ROOT / "lib" / "_internal" / "toml_write.py"


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


class TomlValueTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(TOML_WRITE_PATH, "toml_write_under_test")

    def tv(self, v):
        return self.mod.toml_value(v)

    def test_bool_is_lowercase(self):
        self.assertEqual(self.tv(True), "true")
        self.assertEqual(self.tv(False), "false")

    def test_bool_not_serialized_as_int(self):
        # bool is a subclass of int; must not collapse to "1"/"0".
        self.assertEqual(self.tv(True), "true")
        self.assertNotEqual(self.tv(True), "1")

    def test_int_and_float(self):
        self.assertEqual(self.tv(3), "3")
        self.assertEqual(self.tv(2.5), "2.5")

    def test_string_is_double_quoted(self):
        self.assertEqual(self.tv("hello"), '"hello"')
        # Embedded quotes must be escaped to stay valid TOML.
        self.assertEqual(self.tv('a"b'), '"a\\"b"')

    def test_list_of_strings(self):
        self.assertEqual(self.tv(["a", "b"]), '["a", "b"]')

    def test_nested_and_dict_roundtrip_via_tomllib(self):
        rendered = f"value = {self.tv({'k': True, 'tags': ['x', 'y']})}"
        parsed = tomllib.loads(rendered)
        self.assertEqual(parsed["value"], {"k": True, "tags": ["x", "y"]})

    def test_unsupported_type_raises(self):
        with self.assertRaises(TypeError):
            self.tv(object())


if __name__ == "__main__":
    unittest.main()
