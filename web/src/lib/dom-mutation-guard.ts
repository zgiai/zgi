'use client';

type GuardedNodePrototype = Node & {
  __zgiDomMutationGuardInstalled?: boolean;
};

export function installDomMutationGuard() {
  if (typeof window === 'undefined' || typeof Node === 'undefined') {
    return;
  }

  const nodePrototype = Node.prototype as GuardedNodePrototype;

  if (nodePrototype.__zgiDomMutationGuardInstalled) {
    return;
  }

  const originalInsertBefore = Node.prototype.insertBefore;
  const originalRemoveChild = Node.prototype.removeChild;

  // Browser translators and extensions can move text nodes that React still references.
  Node.prototype.insertBefore = function <T extends Node>(
    this: Node,
    newNode: T,
    referenceNode: Node | null
  ): T {
    if (referenceNode && referenceNode.parentNode !== this) {
      return this.appendChild(newNode) as T;
    }

    return originalInsertBefore.call(this, newNode, referenceNode) as T;
  };

  Node.prototype.removeChild = function <T extends Node>(this: Node, child: T): T {
    if (child.parentNode !== this) {
      return child;
    }

    return originalRemoveChild.call(this, child) as T;
  };

  nodePrototype.__zgiDomMutationGuardInstalled = true;
}
