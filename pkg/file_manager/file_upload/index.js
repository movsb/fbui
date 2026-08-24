const form = document.querySelector('#upload-form');
const button = form.querySelector('button[type="submit"]');
const progress = document.querySelector('#progress');
const progressBar = document.querySelector('#progress-bar');
const progressText = document.querySelector('#progress-text');

form.addEventListener('submit', (event) => {
	event.preventDefault();
	button.disabled = true;
	button.textContent = '上传中…';
	progress.style.display = 'block';
	setProgress(0);

	const request = new XMLHttpRequest();
	request.open('POST', '/');
	request.upload.addEventListener('progress', (upload) => {
		if (upload.lengthComputable) {
			setProgress(Math.round(upload.loaded / upload.total * 100));
		}
	});
	request.addEventListener('load', () => {
		if (request.status >= 200 && request.status < 300) {
			setProgress(100);
			alert('上传成功');
			form.reset();
		} else {
			alert(`上传失败：${request.responseText}`);
		}
		finishUpload();
	});
	request.addEventListener('error', () => {
		alert('上传失败：网络连接已中断');
		finishUpload();
	});
	request.send(new FormData(form));
});

function setProgress(value) {
	progressBar.style.width = `${value}%`;
	progressText.textContent = `${value}%`;
}

function finishUpload() {
	button.disabled = false;
	button.textContent = '上传';
}
