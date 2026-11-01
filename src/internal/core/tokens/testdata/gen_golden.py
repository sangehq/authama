"""Regenerate golden.json from the Python auth-token-manager, read-only.

    ATM=/path/to/auth-token-manager
    PYTHONPATH=$ATM/src PYTHONDONTWRITEBYTECODE=1 HOME=$(mktemp -d) \
        $ATM/.venv/bin/python gen_golden.py > golden.json

Every case is pure text in, text out: nothing here reads or writes a real
credential file (inventory and paths are not exercised; their logic is
covered by the ported unit tests). The Go tests replay every case.
"""

import json
import os
import sys

from auth_token_manager import core
from auth_token_manager.core import AuthTokenManager
from auth_token_manager import platform as plat
from auth_token_manager import aliases

SETS = {
    "template": None,
    "mixed": [
        {"target": "composer", "host": "github.com", "kind": "token", "mechanism": "github-oauth", "token": "ghp_x"},
        {"target": "composer", "host": "repo.magento.com", "kind": "basic", "mechanism": "http-basic", "username": "pub", "password": "pri"},
        {"target": "composer", "host": "bitbucket.org", "kind": "pair", "mechanism": "bitbucket-oauth", "key": "k", "secret": "s"},
        {"target": "composer", "host": "r.example.org", "kind": "headers", "mechanism": "custom-headers", "headers": ["A: b", "Caf\u00e9: \u2603 \U0001F600"]},
        {"target": "npm", "host": "https://npm.pkg.github.com/", "kind": "token", "token": "t1", "scope": "@org", "always_auth": True},
        {"target": "npm", "host": "registry.npmjs.org", "kind": "token", "token": "t2"},
        {"target": "docker", "host": "ghcr.io", "kind": "basic", "username": "u", "password": "p:w"},
        {"target": "git", "host": "github.com", "kind": "basic", "username": "me@x", "password": "p/w:?#[]@!$&'()*+,;= %~"},
        {"target": "git", "host": "http://git.acme:8443/org/", "kind": "basic", "username": "u", "password": "p"},
        {"target": "netrc", "host": "api.heroku.com", "kind": "basic", "username": "l", "password": "k"},
    ],
    "replace": [
        {"target": "composer", "host": "github.com", "kind": "token", "mechanism": "github-oauth", "token": "old"},
        {"target": "composer", "host": "github.com", "kind": "token", "mechanism": "github-oauth", "token": "new"},
        {"target": "npm", "host": "registry.npmjs.org", "kind": "token", "token": "n"},
        {"target": "git", "host": "GitHub.com", "kind": "basic", "username": "u", "password": "p"},
        {"target": "netrc", "host": "h1", "kind": "basic", "username": "u", "password": "p"},
        {"target": "docker", "host": "docker.io", "kind": "basic", "username": "u", "password": "p"},
    ],
}

EXISTING = {
    "composer": [
        None,
        "",
        "   \n",
        '{"github-oauth": {"other.com": "keep", "github.com": "old"}, "zeta": 1, "http-basic": "notadict", "config": {"x": [1, 2.5, true, null]}}',
        '{"gitlab-token": {}, "bearer": {"a": "b"}}',
    ],
    "docker": [
        None,
        '{"credsStore": "desktop", "auths": {"ghcr.io": {"auth": "b2xk"}, "quay.io": {}}, "proxies": {"default": {"httpProxy": "http://p"}}}',
        '{"auths": "weird", "x": "\u00e9"}',
    ],
    "npm": [
        None,
        "registry=https://registry.npmjs.org/\n//registry.npmjs.org/:_authToken=old\n; comment\n# c\n\n@org:registry=https://old/\n//npm.pkg.github.com/:always-auth=true\nsave-exact=true\n",
        "\n\n//x/:_authToken=y\n\n",
    ],
    "git": [
        None,
        "https://u:p@github.com\n# comment\n\nhttps://a:b@gitlab.com\nhttps://u:p@GITHUB.COM/\nhttps://u:p@git.acme:8443/org\nhttps://u:p@git.acme:bad/org\nhttps://u:p@[::1]:99/x\n",
    ],
    "netrc": [
        None,
        "machine h1 login a password b\nmachine h2 login c password d machine h3 login e password f\ndefault login anon password x\n",
    ],
}


def build(name):
    if SETS[name] is None:
        return AuthTokenManager.template()
    m = AuthTokenManager()
    for e in SETS[name]:
        m._add(core.Credential(**e))
    return m


def renders():
    out = []
    for name in SETS:
        m = build(name)
        for target, texts in EXISTING.items():
            for text in texts:
                mm = build(name)
                try:
                    got = mm.render(target, text)
                except Exception as exc:  # noqa: BLE001
                    got = "ERROR: " + str(exc)
                out.append({"set": name, "target": target, "existing": text, "out": got, "warnings": mm.warnings})
        out.append({"set": name, "target": "_json", "existing": None, "out": m.to_json(), "warnings": []})
        out.append({"set": name, "target": "_composer_env", "existing": None, "out": m.to_composer_env(), "warnings": []})
    return out


SCAN_TEXTS = {
    "composer": EXISTING["composer"][3:] + [
        '{"http-basic": {"h": {"username": "u", "password": ""}, "b": {"consumer-key": "k", "consumer-secret": "s"}, "n": {"username": 5, "password": true}}, "custom-headers": {"c": ["a", 1]}, "github-oauth": {"e": ""}}',
        "not json", "[1]",
    ],
    "npm": EXISTING["npm"][1:] + [
        "//r1/:_auth=dTpw\n//r2/:username=bob\n//r2/:_password=c2VjcmV0\n//r3/:_auth=!!!\n//r4/:_password=bad=\n//r5:_authToken= spaced \n",
    ],
    "docker": EXISTING["docker"][1:] + [
        '{"auths": {"a": {"auth": "dTpwOng="}, "b": {"identitytoken": "idt"}, "c": {"auth": "%%%"}, "d": "s"}}',
        "nope",
    ],
    "git": EXISTING["git"][1:] + [
        "github.com\nhttps://%E2%9C%93:p%2Fw%ZZ@h.example/x/\nftp://u@h2\nhttps://u:@h3\nhttps://u:p@[bad]/x\nhttps://u:p@h4:0/\nhttps://u:p@h5:070000\n",
    ],
    "netrc": EXISTING["netrc"][1:] + [
        "machine m1 login\nmachine m2 password p login l account a\nmacdef init\ncd /\n\nmachine m3 login x password y\nmachine",
    ],
}


def scans():
    return [{"target": t, "text": x, "out": core._SCANNERS[t](x)} for t, xs in SCAN_TEXTS.items() for x in xs]


def purges():
    out = []
    hosts = {
        "composer": ["github.com", "other.com", "a"],
        "docker": ["ghcr.io", "quay.io", "a"],
        "npm": ["registry.npmjs.org", "https://old/", "npm.pkg.github.com"],
        "git": ["github.com", "https://GitHub.com/", "git.acme:8443/org", "git.acme:bad/org"],
        "netrc": ["h1", "h2", "m3"],
    }
    for t, xs in SCAN_TEXTS.items():
        for x in xs:
            for h in hosts[t]:
                out.append({"target": t, "host": h, "text": x, "out": core._purge(t, h, x)})
    return out


def misc():
    vals = ["github.com", "GitHub.com/", "https://u:p@Git.Acme:8443/Org/", "git.acme:bad", "h:0", "h:65536",
            "dev.azure.com/org", "[::1]:22", "https://[v1.x]/a", "ssh://h?x#y", "u@h", "HTTPS://H/p"]
    shells = ["posix", "fish", "powershell", "cmd"]
    values = ['{"a":"b"}', "it's", "a\\b'c", "plain", "x&y|z<>^()"]
    describe = [(t, h) for t in core.TARGETS for h in ["github.com", "github.com/org", "gitlab.com", "codeberg.org",
                "dev.azure.com", "https://index.docker.io/v1/", "registry.npmjs.org", "nowhere.example", "ghcr.io"]]
    return {
        "gitkey": {v: core._git_host_of(v) for v in vals},
        "quote": {v: core._url_quote(v) for v in ["a b", "\u00e9", "~-._", "a/b@c"]},
        "env_export": [{"shell": s, "value": v, "out": plat.env_export("X", v, s)} for s in shells for v in values],
        "describe": [{"target": t, "host": h, "out": core.describe(t, h, "")} for t, h in describe],
        "alias_block": aliases.build_block(aliases.default_aliases("authama")),
        "strip_block": [{"text": x, "install": aliases.render_install(x, "BLOCK"), "remove": aliases.render_remove(x)} for x in [
            "", "a\n", "a\n\n\n# >>> auth-token-manager >>>\nold\n# <<< auth-token-manager <<<\nb\n", "x\n# >>> auth-token-manager >>>\nunterminated"]],
    }


def configs():
    env = {"GH": " tok\n", "U": "user", "P": "pass", "EMPTY": "", "K": "k", "S": "s", "HV": "hv"}
    docs = [
        {"composer": {"github.com": {"service": "github", "env": "GH"}, "x.org": {"service": "nope"}, "y.org": {},
                      "z.org": {"type": "weird", "env": "GH"}, "m.org": {"service": "magento", "user_env": "U", "pass_env": "P"},
                      "b.org": {"type": "bitbucket-oauth", "key_env": "K", "secret_env": "S"},
                      "h.org": {"type": "custom-headers", "headers": ["X: ${HV}", "E: ${EMPTY}"]},
                      "h2.org": {"type": "custom-headers", "headers": ["X: ${MISSING}"]},
                      "e.org": {"service": "github", "env": "EMPTY"}},
         "npm": {"r": {"env": "GH", "scope": "@o", "always_auth": True}, "r2": {"env": "MISSING"}},
         "docker": {"d": {"user_env": "U", "pass_env": "P"}}, "git": {"g": {"user_env": "U", "pass_env": "MISSING"}},
         "netrc": {"n": {"user_env": "U", "pass_env": "P"}}, "_help": "ignored"},
        {"credentials": {"github.com": {"service": "github", "env": "GH"}, "_c": {}}},
        {"github.com": {"service": "github", "env": "GH"}, "_note": "x"},
        {"credentials": "bad"},
        {"composer": {"h%d" % i: {"service": "github", "env": "MISSING"} for i in range(8)}},
    ]
    old = dict(os.environ)
    out = []
    try:
        for k in list(os.environ):
            if k in env or k == "MISSING":
                del os.environ[k]
        os.environ.update(env)
        for d in docs:
            m = AuthTokenManager.from_config(d)
            out.append({"doc": d, "warnings": m.warnings,
                        "renders": {t: m.render(t, None) for t in m.targets()}})
    finally:
        os.environ.clear()
        os.environ.update(old)
    return {"env": env, "cases": out}


json.dump({"source": "simtabi/auth-token-manager", "renders": renders(), "scans": scans(), "purges": purges(),
           "misc": misc(), "configs": configs()}, sys.stdout, indent=1, ensure_ascii=False)
sys.stdout.write("\n")
