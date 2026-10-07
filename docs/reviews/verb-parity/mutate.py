#!/usr/bin/env python3
"""Prove method-shorthand fixtures reject a physically broken parser; restore always."""
import pathlib
import subprocess

root=pathlib.Path(__file__).resolve().parents[3]
source=root/'tools/verb_parity.py'
original=source.read_text()
anchor="methods = env.get('method', {'<id>'})"
assert original.count(anchor)==1
try:
    source.write_text(original.replace(anchor,"methods = {'GET'}"))
    result=subprocess.run(['python3','-m','unittest','discover','-s','tools','-p','test_verb_parity.py','-v'],cwd=root,text=True,capture_output=True)
    assert result.returncode!=0 and 'FAIL: test_js_templates_concatenation_and_method_shorthand' in result.stderr and 'AssertionError' in result.stderr, result.stderr
    print('dynamic PUT/DELETE -> GET: named fixture assertion red')
finally:
    source.write_text(original)
subprocess.run(['python3','-m','unittest','discover','-s','tools','-p','test_verb_parity.py'],cwd=root,check=True)
print('restored fixtures green')
