#!/usr/bin/env python3
# coding: utf-8

import json
import os
import subprocess

class Bookmark:
    def __init__(self, path, line, annotation=None):
        self.path = path
        self.line = line
        self.annotation = annotation

def extract_bookmarks(filepath='.vim-bookmarks'):
    lines = list(open(filepath))
    for path, bookmarks in eval(lines[1].split('=',1)[1])['default'].items():
        for bookmark in bookmarks:
            # TODO: Makes paths relative to the repo root.
            yield Bookmark(os.path.relpath(path), bookmark.get('line_nr', None), bookmark.get('annotation', None))

def print_bookmarks(bookmarks):
    filename = None
    for bookmark in sorted(bookmarks, key=lambda b: (b.path, b.line)):
        if filename != bookmark.path:
            if filename is not None:
                print()
            print(f"{bookmark.path}:")
            filename = bookmark.path

        print(f"\t{bookmark.line}: {bookmark.annotation}")
    print()

def confirm(question, default='no'):
    if default is None:
        prompt = " [y/n] "
    elif default == 'yes':
        prompt = " [Y/n] "
    elif default == 'no':
        prompt = " [y/N] "
    else:
        raise ValueError(f"Unknown setting '{default}' for default.")

    while True:
        resp = input(question + prompt).strip().lower()
        if resp in {'yes', 'y', 'n', 'no'}:
            return resp.startswith('y')
        if default is not None and resp == '':
            return default == 'yes'
        print("Please respond with y(es) or n(o).\n")

def main():
    bookmarks = list(extract_bookmarks())

    print_bookmarks(bookmarks)
    commit = subprocess.check_output(['git', 'rev-parse', 'HEAD']).decode().strip()
    print(f"Commit: {commit}")
    pr = subprocess.check_output(['hub', 'pr', 'show', '-f', '%I']).decode().strip()
    print(f"Pull request: {pr}")

    if not confirm("Post to GitHub?"):
        return

    # Check to see if a pending review already exists.
    reviews = json.loads(subprocess.check_output([
        'hub', 'api', f'/repos/{{owner}}/{{repo}}/pulls/{pr}/reviews',
    ]))
    # TODO: Make the username variable.
    if any(r['user']['login'] == 'nategraf' and r['state'] == 'PENDING' for r in reviews):
        if not confirm("Append to existing pending review?"):
            return
    else:
        # Create a new pending review to which comments will be added.
        subprocess.check_call([
            'hub', 'api', f'/repos/{{owner}}/{{repo}}/pulls/{pr}/reviews',
            '-f', f'commit_id={commit}',
        ])

    for bookmark in bookmarks:
        subprocess.check_call([
            'hub', 'api', f'/repos/{{owner}}/{{repo}}/pulls/{pr}/comments',
            '-H', 'Accept:application/vnd.github.comfort-fade-preview+json',
            '-f', f'commit_id={commit}',
            '-f', f'path={bookmark.path}',
            '-f', 'side=RIGHT',
            '-F', f'line={bookmark.line}',
            '-f', f'body={bookmark.annotation}'
        ])

if __name__ == "__main__":
    main()
