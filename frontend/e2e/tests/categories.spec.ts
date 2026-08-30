import { test, expect } from '../fixtures';
import { sel } from '../utils/selectors';

test.describe('Categories', () => {
  test('can create a new category', async ({ page, gotoApp }) => {
    await gotoApp();

    // Open context menu on the sidebar content area (empty space)
    const sidebarContent = page.locator('.sidebar-content');
    await sidebarContent.click({ button: 'right' });

    // Radix context menu renders as portal with role="menu"
    await expect(page.locator('[role="menuitem"]').filter({ hasText: /New Category/ })).toBeVisible();
    await page.locator('[role="menuitem"]').filter({ hasText: /New Category/ }).click();

    // Category editor modal should appear
    await expect(page.locator(sel.categoryEditor)).toBeVisible();

    // Fill in name
    await page.locator(sel.categoryNameInput).fill('Test Category');

    // Click save (the last button in the dialog)
    const dialog = page.locator(sel.categoryEditor);
    await dialog.locator('button').filter({ hasText: 'Create' }).click();

    // Modal should close
    await expect(page.locator(sel.categoryEditor)).not.toBeVisible();

    // New category should appear in sidebar
    await expect(page.getByText('Test Category')).toBeVisible();
  });

  test('can edit a seeded category', async ({ page, seed, gotoApp }) => {
    const catId = 'cat-edit-test';
    await seed({
      categories: [
        {
          id: catId,
          name: 'Original Name',
          icon: '',
          color: '#ff0000',
          createdAt: new Date().toISOString(),
          updatedAt: new Date().toISOString(),
        },
      ],
    });
    await gotoApp();

    // Verify category exists
    await expect(page.locator('.text-sm', { hasText: 'Original Name' })).toBeVisible();

    // Right-click category header
    const catHeader = page.locator('.sidebar-section-header').first();
    await catHeader.click({ button: 'right' });

    // Click on "Edit Category" in context menu — uses role="menuitem"
    await expect(page.locator('[role="menuitem"]').filter({ hasText: 'Edit Category' })).toBeVisible();
    await page.locator('[role="menuitem"]').filter({ hasText: 'Edit Category' }).click();

    // Category editor modal should appear
    await expect(page.locator(sel.categoryEditor)).toBeVisible();

    // Change the name
    const nameInput = page.locator(sel.categoryNameInput);
    await nameInput.fill('Updated Name');

    // Save
    const dialog = page.locator(sel.categoryEditor);
    await dialog.locator('button').filter({ hasText: 'Save' }).click();

    // Verify updated name in sidebar
    await expect(page.getByText('Updated Name')).toBeVisible();
  });

  test('can delete a seeded category', async ({ page, seed, gotoApp }) => {
    const catId = 'cat-del-test';
    await seed({
      categories: [
        {
          id: catId,
          name: 'Delete Me',
          icon: '',
          color: '#000',
          createdAt: new Date().toISOString(),
          updatedAt: new Date().toISOString(),
        },
      ],
    });
    await gotoApp();

    // Verify category exists
    await expect(page.locator('.text-sm', { hasText: 'Delete Me' })).toBeVisible();

    // Right-click category header
    const catHeader = page.locator('.sidebar-section-header').first();
    await catHeader.click({ button: 'right' });

    // Click on "Delete" option (text-destructive menuitem)
    await expect(page.locator('[role="menuitem"]').filter({ hasText: 'Delete' })).toBeVisible();
    await page.locator('[role="menuitem"]').filter({ hasText: 'Delete' }).click();

    // Confirmation dialog should appear
    await expect(page.locator(sel.confirmDeleteCategoryDialog)).toBeVisible();

    // Confirm deletion
    await page.locator(sel.confirmDeleteCategoryConfirm).click();

    // Category should be gone
    await expect(page.getByText('Delete Me')).not.toBeVisible();
  });

  test('cancelling the delete-category dialog keeps the category', async ({ page, seed, gotoApp }) => {
    const catId = 'cat-cancel-test';
    await seed({
      categories: [
        {
          id: catId,
          name: 'Keep Me',
          icon: '',
          color: '#000',
          createdAt: new Date().toISOString(),
          updatedAt: new Date().toISOString(),
        },
      ],
    });
    await gotoApp();

    const catHeader = page.locator('.sidebar-section-header').first();
    await catHeader.click({ button: 'right' });
    await page.locator('[role="menuitem"]').filter({ hasText: 'Delete' }).click();

    await expect(page.locator(sel.confirmDeleteCategoryDialog)).toBeVisible();
    await page.locator(sel.confirmDeleteCategoryCancel).click();

    await expect(page.locator(sel.confirmDeleteCategoryDialog)).not.toBeVisible();
    await expect(page.getByText('Keep Me')).toBeVisible();
  });
});
