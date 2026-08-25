const editor = document.querySelector('#editor');
const save = document.querySelector('#save');

async function load() {
	try {
		const response = await fetch('/api/file');
		if (!response.ok) {
			throw new Error(await response.text());
		}
		editor.value = await response.text();
	} catch (error) {
		alert(`读取失败：${error.message}`);
	}
}

save.addEventListener('click', async () => {
	save.disabled = true;
	save.textContent = '保存中…';
	try {
		const response = await fetch('/api/file', {
			method: 'PUT',
			body: editor.value,
		});
		if (!response.ok) {
			throw new Error(await response.text());
		}
		alert('保存成功');
		save.textContent = '保存';
	} catch (error) {
		alert(`保存失败：${error.message}`);
		save.disabled = false;
		save.textContent = '保存';
	}
});

load();
