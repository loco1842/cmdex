import { describe, it, expect } from 'vitest';
import {
  extractTemplateVarNames,
  mergeDetectedVariables,
  buildVariablesFromScript,
  normalizeVariablesForCompare,
  variableDefinitionsToPrompts,
} from './templateVars';
import type { VariableDefinition } from '../types';

function variable(overrides: Partial<VariableDefinition> = {}): VariableDefinition {
  return { name: 'foo', description: '', example: '', default: '', sortOrder: 0, ...overrides };
}

describe('extractTemplateVarNames', () => {
  it('detects unique variables in order of first appearance', () => {
    expect(extractTemplateVarNames('echo {{name}} {{name}} {{city}}')).toEqual(['name', 'city']);
  });

  it('matches only word characters — no dots, dashes, or internal spaces', () => {
    // The regex is \{\{(\w+)\}\} (templateVars.ts:3): dashes and dots are not
    // \w, so these template-looking strings are deliberately NOT variables.
    expect(extractTemplateVarNames('{{my-var}}')).toEqual([]);
    expect(extractTemplateVarNames('{{a.b}}')).toEqual([]);
    expect(extractTemplateVarNames('{{ x }}')).toEqual([]);
  });

  it('matches plain alphanumeric/underscore names', () => {
    expect(extractTemplateVarNames('{{my_var}} {{Var2}}')).toEqual(['my_var', 'Var2']);
  });

  it('returns an empty array for a script with no variables', () => {
    expect(extractTemplateVarNames('echo hello')).toEqual([]);
  });
});

describe('mergeDetectedVariables vs buildVariablesFromScript — documented asymmetry', () => {
  // mergeDetectedVariables (used on load, App.tsx draftFromCommand path) keeps
  // variables no longer referenced by the script ("orphans"). buildVariablesFromScript
  // (used on save) drops them. This pair of tests pins that intentional asymmetry.
  const orphan = variable({ name: 'orphan', default: 'kept-if-merge' });

  it('mergeDetectedVariables keeps an orphaned variable not present in the script', () => {
    const result = mergeDetectedVariables('echo {{name}}', [orphan]);
    const names = result.map((v) => v.name);
    expect(names).toContain('orphan');
    expect(names).toContain('name');
  });

  it('buildVariablesFromScript drops the same orphaned variable', () => {
    const result = buildVariablesFromScript('echo {{name}}', [orphan]);
    const names = result.map((v) => v.name);
    expect(names).not.toContain('orphan');
    expect(names).toEqual(['name']);
  });

  it('mergeDetectedVariables preserves existing metadata for a still-referenced variable', () => {
    const existing = variable({ name: 'name', default: 'World', description: 'greeting target' });
    const result = mergeDetectedVariables('echo {{name}}', [existing]);
    expect(result).toEqual([existing]);
  });

  it('both functions assign fresh metadata for a newly-detected variable', () => {
    const result = buildVariablesFromScript('echo {{brandNew}}', []);
    expect(result).toEqual([{ name: 'brandNew', description: '', example: '', default: '', sortOrder: 0 }]);
  });
});

describe('normalizeVariablesForCompare', () => {
  it('drops sortOrder, keeping only name/description/default/example', () => {
    const result = normalizeVariablesForCompare([variable({ sortOrder: 7, default: 'x' })]);
    expect(result).toEqual([{ name: 'foo', description: '', default: 'x', example: '' }]);
  });
});

describe('variableDefinitionsToPrompts', () => {
  it('maps a VariableDefinition to a VariablePrompt, using name as placeholder', () => {
    const result = variableDefinitionsToPrompts([variable({ name: 'city', default: 'NYC' })]);
    expect(result).toEqual([
      { name: 'city', placeholder: 'city', description: '', example: '', defaultExpr: 'NYC', defaultValue: 'NYC' },
    ]);
  });

  it('defaults defaultValue to empty string when default is falsy', () => {
    const result = variableDefinitionsToPrompts([variable({ name: 'x', default: '' })]);
    expect(result[0].defaultValue).toBe('');
  });
});
