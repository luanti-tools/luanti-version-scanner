#!/usr/bin/env python3
"""
Generate db.json from Luanti's lua_api.md git history.

For each API, finds the earliest release tag where it's documented.
Handles both markdown (5.8.0+) and plaintext (pre-5.8.0) formats.
"""
import json
import os
import re
import subprocess
import sys
from collections import defaultdict

LUANTI_REPO = "/home/mcginnis/Documents/Git/luanti"
MOD_VERSION_DIR = "/home/mcginnis/Documents/Git/mod_version"

def run_git(cmd):
    return subprocess.check_output(cmd, cwd=LUANTI_REPO, text=True).strip()

def get_tags():
    tags = run_git(["git", "tag", "-l"]).split("\n")
    stable = [t for t in tags if re.match(r'^\d+\.\d+\.\d+$', t) and 'rc' not in t]
    stable.sort(key=lambda x: [int(p) for p in x.split('.')])
    return stable

def get_doc_at_tag(tag):
    for fname in ["doc/lua_api.md", "doc/lua_api.txt"]:
        try:
            content = run_git(["git", "show", f"{tag}:{fname}"])
            return content, fname.endswith(".md")
        except:
            continue
    return None, False

def is_version_tag(tag):
    return bool(re.match(r'^\d+\.\d+\.\d+$', tag))

def find_func_in_doc(content, name, is_md):
    """Check if a function name appears as a documented API in a doc version.
    
    Handles multiple documentation formats:
    - Markdown (5.8.0+):   * `core.func(args)`
    - Plaintext (0.4.12+): * `core.func(args)`
    - Plaintext (0.4.0-0.4.11):   core.func(args) or * core.func(args)
    """
    patterns = []
    
    core_name = None
    minetest_name = None
    short_name = name
    colon_name = None
    
    if name.startswith("minetest."):
        short_name = name[9:]
        core_name = "core." + short_name
        minetest_name = name
    elif name.startswith("core."):
        short_name = name[5:]
        core_name = name
        minetest_name = "minetest." + short_name
    elif ":" in name:
        parts = name.split(":", 1)
        colon_name = name
        short_name = parts[1]
    
    # Dot-notation variant for colon-style APIs (early docs used object.method)
    dot_name = colon_name.replace(":", ".") if colon_name else None
    dot_short = short_name  # same
    
    # Markdown/plaintext with asterisk and backtick: * `prefix.func(`
    for pname in [n for n in [core_name, minetest_name, name] if n]:
        patterns.append(r'\*\s*`' + re.escape(pname) + r'\s*\(')
    
    # Plaintext format with asterisk but no backtick: * prefix.func(
    for pname in [n for n in [core_name, minetest_name, name] if n]:
        patterns.append(r'^\s*\*\s*' + re.escape(pname) + r'\s*\(')
    
    # Plaintext indented format:   prefix.func(args)
    for pname in [n for n in [core_name, minetest_name, name] if n]:
        patterns.append(r'^\s*' + re.escape(pname) + r'\s*\(')
    
    # Short name patterns
    patterns.append(r'\*\s*`' + re.escape(short_name) + r'\s*\(')
    patterns.append(r'^\s*\*\s*' + re.escape(short_name) + r'\s*\(')
    patterns.append(r'^\s*-\s+' + re.escape(short_name) + r'\s*\(')
    patterns.append(r'^\s*' + re.escape(short_name) + r'\s*\(')
    
    # Dot-notation for colon APIs: object.method(
    if dot_name:
        patterns.append(r'^\s*' + re.escape(dot_name) + r'\s*\(')
        patterns.append(r'^\s*-\s+' + re.escape(dot_name) + r'\s*\(')
        patterns.append(r'\*\s*`' + re.escape(dot_name) + r'\s*\(')
    
    for pat in patterns:
        if re.search(pat, content, re.MULTILINE):
            return True
    
    return False

def extract_features_from_current():
    with open(f"{LUANTI_REPO}/doc/lua_api.md") as f:
        content = f.read()
    
    features = {}
    m = re.search(r'core\.features.*?```lua\n(.*?)```', content, re.DOTALL)
    if not m:
        return features
    
    table = m.group(1)
    current_desc = ""
    for line in table.split("\n"):
        line = line.strip()
        if not line:
            continue
        if line.startswith("--"):
            comment_text = line[2:].strip()
            # Check if this comment line has a version in it
            has_ver = False
            for pat in [
                r'\((\d+\.\d+(?:\.\d+)?)\)',
                r'(?<!\w)(\d+\.\d+(?:\.\d+)?)(?!\w)',
            ]:
                if re.search(pat, comment_text):
                    has_ver = True
                    break
            if has_ver and not re.search(r'[a-z]', comment_text, re.I):
                # Version-only comment, reset current_desc
                current_desc = comment_text
            else:
                current_desc = (current_desc + " " + comment_text).strip()
            continue
        
        mm = re.match(r'(\w+)\s*=\s*true,?\s*(?:--\s*(.*))?$', line)
        if mm:
            name = mm.group(1)
            inline_comment = mm.group(2) or ""
            ver = None
            for text in [inline_comment, current_desc]:
                for pat in [
                    r'\((\d+\.\d+(?:\.\d+)?)\)',
                    r'(?<!\w)(\d+\.\d+(?:\.\d+)?)(?!\w)',
                ]:
                    ver_m = re.search(pat, text)
                    if ver_m:
                        v = ver_m.group(1)
                        parts = v.split(".")
                        if len(parts) >= 2:
                            try:
                                int(parts[0])
                                int(parts[1])
                                ver = v
                            except:
                                pass
                            break
                if ver:
                    break
            if ver:
                desc = ""
                if inline_comment:
                    desc = re.sub(r'\s*\(\s*\d+\.\d+(?:\.\d+)?\s*\)\s*$', '', inline_comment).strip()
                    if not desc or desc == inline_comment:
                        desc = re.sub(r'\s*\d+\.\d+(?:\.\d+)?\s*$', '', inline_comment).strip()
                elif current_desc:
                    desc = re.sub(r'\s*\(\s*\d+\.\d+(?:\.\d+)?\s*\)\s*$', '', current_desc).strip()
                    if not desc or desc == current_desc:
                        desc = re.sub(r'\s*\d+\.\d+(?:\.\d+)?\s*$', '', current_desc).strip()
                features[name] = {"Version": ver, "//": desc}
            current_desc = ""
    
    return features

def extract_params_from_current():
    """Extract parameter definitions from current lua_api.md."""
    with open(f"{LUANTI_REPO}/doc/lua_api.md") as f:
        content = f.read()
    
    lines = content.split("\n")
    params = {}
    
    for i, line in enumerate(lines):
        m = re.match(r'\s*\*\s*`(?:core\.|minetest\.)?(\w+)\s*\(([^)]*)\)', line)
        if not m:
            continue
        
        func_name = m.group(1)
        func_params = {}
        
        j = i + 1
        while j < len(lines) and j < i + 30:
            nl = lines[j]
            if re.match(r'\s*\*\s*`\w+\s*\(', nl):
                break
            if re.match(r'^#{1,4}\s+', nl):
                break
            
            pm = re.search(r'`(\w+)`\s*:', nl)
            if not pm:
                j += 1
                continue
            
            param_name = pm.group(1)
            if param_name in ("true", "false", "nil", "self"):
                j += 1
                continue
            
            ver = None
            for pat in [
                r'(?:since|added in|introduced in|Available since)\s+(?:version\s+)?(\d+\.\d+(?:\.\d+)?)',
                r'\((\d+\.\d+(?:\.\d+)?)\)',
            ]:
                vm = re.search(pat, nl, re.I)
                if vm:
                    v = vm.group(1)
                    parts = v.split(".")
                    if len(parts) >= 2:
                        try:
                            int(parts[0])
                            int(parts[1])
                            ver = v
                        except:
                            pass
                        break
            
            if ver:
                func_params[param_name] = {"Version": ver}
            
            j += 1
        
        if func_params:
            params[func_name] = func_params
    
    return params

def main():
    tags = get_tags()
    
    existing_path = f"{MOD_VERSION_DIR}/internal/db/db.json"
    with open(existing_path) as f:
        existing = json.load(f)
    
    existing_apis = existing.get("apis", {})
    existing_features = existing.get("features", {})
    
    print(f"Starting with {len(existing_apis)} APIs, {len(existing_features)} features")
    print(f"Scanning {len(tags)} tags for version discovery...")
    
    # For each API, find the earliest tag where it appears as a documented function
    api_first_seen = {}
    api_last_seen = {}
    
    # Also track API presence per tag for deprecation detection
    api_in_tag = defaultdict(set)
    
    for idx, tag in enumerate(tags):
        if not is_version_tag(tag):
            continue
        
        content, is_md = get_doc_at_tag(tag)
        if content is None:
            continue
        
        # Check each API against this tag's docs
        for api_name in existing_apis:
            if find_func_in_doc(content, api_name, is_md):
                api_in_tag[tag].add(api_name)
                if api_name not in api_first_seen:
                    api_first_seen[api_name] = tag
                api_last_seen[api_name] = tag
        
        if (idx + 1) % 10 == 0:
            print(f"  Processed {tag} ({len(api_first_seen)}/{len(existing_apis)} found so far)...")
    
    print(f"Found versions for {len(api_first_seen)}/{len(existing_apis)} APIs")
    
    # Build new APIs dict
    new_apis = {}
    versions_changed = 0
    max_versions_added = 0
    
    for api_name in existing_apis:
        entry = {}
        
        # Keep existing comment
        if "//" in existing_apis[api_name]:
            entry["//"] = existing_apis[api_name]["//"]
        
        # Set version
        old_ver = existing_apis[api_name].get("Version", "")
        if api_name in api_first_seen:
            entry["Version"] = api_first_seen[api_name]
            if old_ver and old_ver != api_first_seen[api_name]:
                versions_changed += 1
        else:
            entry["Version"] = old_ver
        
        # Detect deprecation: if API was last seen in a tag that isn't the latest
        latest_tag = tags[-1] if tags else ""
        if api_name in api_last_seen and api_last_seen[api_name] != latest_tag:
            # Find the tag after last_seen that does NOT have this function
            last_seen_idx = tags.index(api_last_seen[api_name])
            for next_tag in tags[last_seen_idx + 1:]:
                if next_tag in api_in_tag and api_name not in api_in_tag[next_tag]:
                    entry["MaxVersion"] = next_tag
                    max_versions_added += 1
                    break
        
        # Keep existing MaxVersion if we didn't find one
        if "MaxVersion" not in entry and "MaxVersion" in existing_apis[api_name]:
            entry["MaxVersion"] = existing_apis[api_name]["MaxVersion"]
        
        new_apis[api_name] = entry
    
    # Extract features
    new_features = extract_features_from_current()
    print(f"Extracted {len(new_features)} features from current docs")
    
    # Merge existing features not in parsed feature list
    for fname, fentry in existing_features.items():
        if fname not in new_features:
            new_features[fname] = fentry
    
    # Extract params
    doc_params = extract_params_from_current()
    for api_name, entry in new_apis.items():
        short_name = api_name
        if api_name.startswith("minetest."):
            short_name = api_name[9:]
        elif ":" in api_name:
            short_name = api_name.split(":")[1]
        if short_name in doc_params:
            entry["Params"] = doc_params[short_name]
        elif "Params" in existing_apis.get(api_name, {}):
            entry["Params"] = existing_apis[api_name]["Params"]
    
    # Sort
    sorted_apis = dict(sorted(new_apis.items()))
    sorted_features = dict(sorted(new_features.items()))
    
    output = {"apis": sorted_apis, "features": sorted_features}
    
    output_path = f"{MOD_VERSION_DIR}/internal/db/db.json"
    with open(output_path, "w") as f:
        json.dump(output, f, indent=2, ensure_ascii=False)
    
    not_found = sum(1 for name in existing_apis if name not in api_first_seen)
    
    print(f"\nGenerated {output_path}")
    print(f"  APIs: {len(sorted_apis)} ({versions_changed} versions updated, {max_versions_added} MaxVersions added, {not_found} not found in docs)")
    print(f"  Features: {len(sorted_features)}")
    
    if versions_changed > 0:
        print("\nSample version changes:")
        count = 0
        for name in existing_apis:
            if name in api_first_seen and existing_apis[name].get("Version", "") and existing_apis[name]["Version"] != api_first_seen[name]:
                print(f"  {name}: {existing_apis[name]['Version']} -> {api_first_seen[name]}")
                count += 1
                if count >= 15:
                    break

if __name__ == "__main__":
    main()
