#include <stdio.h>
#include <stdlib.h>
#include <stdint.h>
#include <unistd.h>
#include <fcntl.h>
#include <signal.h>
#include <string.h>
#include <sys/ioctl.h>

static volatile sig_atomic_t quit = 0;

static void onSignal(int sig) {
	(void)sig;
	quit = 1;
}

typedef struct TPlayer TPlayer;

typedef enum {
	CEDARX_PLAYER = 0,
	GSTREAMER_PLAYER = 1,
} TplayerType;

typedef int (*TPlayerNotifyCallback)(
	void *pUserData,
	int msg,
	int ext1,
	void *param
);

extern TPlayer *TPlayerCreate(TplayerType type);
extern void TPlayerDestroy(TPlayer *p);

extern int TPlayerSetNotifyCallback(
	TPlayer *p,
	TPlayerNotifyCallback notifier,
	void *pUserData
);

extern int TPlayerSetDataSource(
	TPlayer *p,
	const char *url,
	void *httpService
);

extern int TPlayerPrepare(TPlayer *p);
extern int TPlayerStart(TPlayer *p);
extern int TPlayerStop(TPlayer *p);

extern int TPlayerSetVideoDisplay(
	TPlayer *p,
	int enable
);

extern int TPlayerSetDisplayRect(
	TPlayer *p,
	int x,
	int y,
	int width,
	int height
);

#define DISP_LAYER_SET_CONFIG 0x47
#define DISP_LAYER_GET_CONFIG 0x48

/*
 * 从官方视频播放器逆向得到。
 *
 * TrimUI 官方 mediaplayer 中：
 *
 * sizeof(disp_layer_config) = 184
 *
 * offset:
 *   info.alpha_mode  = 5
 *   info.alpha_value = 6
 *   enable           = 0xac
 *   channel          = 0xb0
 *   layer_id         = 0xb4
 */
static int configure_ui_layer(int transparent) {
	int fd;
	unsigned char cfg[184];
	unsigned long args[4];

	fd = open("/dev/disp", O_RDWR);
	if (fd < 0) {
		perror("open /dev/disp");
		return -1;
	}

	memset(cfg, 0, sizeof(cfg));

	/*
	 * 这三项完全照官方 mediaplayer。
	 */
	*(uint32_t *)(cfg + 0xac) = 1; /* enable */
	*(uint32_t *)(cfg + 0xb0) = 0; /* channel */
	*(uint32_t *)(cfg + 0xb4) = 0; /* layer_id */

	/*
	 * kernel ABI:
	 *
	 * args[0] = screen id
	 * args[1] = disp_layer_config *
	 * args[2] = config count
	 */
	memset(args, 0, sizeof(args));

	args[0] = 0;
	args[1] = (unsigned long)cfg;
	args[2] = 1;

	if (ioctl(fd, DISP_LAYER_GET_CONFIG, args) < 0) {
		perror("DISP_LAYER_GET_CONFIG");
		close(fd);
		return -1;
	}

	/*
	 * 官方 disp2_ui_transparent():
	 *
	 * transparent != 0:
	 *     alpha_mode = 0    // pixel alpha
	 *
	 * transparent == 0:
	 *     alpha_mode = 1    // global alpha
	 *
	 * alpha_value 始终 255
	 */
	cfg[5] = transparent ? 0 : 1;
	cfg[6] = 255;

	memset(args, 0, sizeof(args));

	args[0] = 0;
	args[1] = (unsigned long)cfg;
	args[2] = 1;

	if (ioctl(fd, DISP_LAYER_SET_CONFIG, args) < 0) {
		perror("DISP_LAYER_SET_CONFIG");
		close(fd);
		return -1;
	}

	close(fd);
	return 0;
}

static int onPlayerNotify(void *userData, int msg, int ext1, void *param) {
	(void)userData;

	/*
	printf(
		"TPlayer notify: msg=%d ext1=%d param=%p\n",
		msg,
		ext1,
		param
	);
	*/

	return 0;
}

static int parseInt(const char *s, const char *name) {
	char *end;
	long v;

	v = strtol(s, &end, 10);

	if (*s == '\0' || *end != '\0') {
		fprintf(stderr, "invalid %s: %s\n", name, s);
		exit(1);
	}

	return (int)v;
}

int main(int argc, char **argv) {
	TPlayer *p = NULL;

	int x;
	int y;
	int width;
	int height;

	if (argc != 6) {
		fprintf(
			stderr,
			"usage: %s video.mp4 x y width height\n"
			"\n"
			"example:\n"
			"  %s video.mp4 100 80 824 600\n"
			"\n"
			"fullscreen:\n"
			"  %s video.mp4 0 0 1024 768\n",
			argv[0],
			argv[0],
			argv[0]
		);
		return 1;
	}

	x      = parseInt(argv[2], "x");
	y      = parseInt(argv[3], "y");
	width  = parseInt(argv[4], "width");
	height = parseInt(argv[5], "height");

	if (width <= 0 || height <= 0) {
		fprintf(stderr, "width and height must be > 0\n");
		return 1;
	}

	signal(SIGINT, onSignal);
	signal(SIGTERM, onSignal);

	/*
	 * 这个就是之前发现的关键步骤。
	 *
	 * 官方播放器启动时会操作 UI layer。
	 * 如果不做这一步，CedarX 的视频 layer 有可能已经在播放，
	 * 但被 UI layer 完全盖住，于是表现为“只有声音没有视频”。
	 */
	if (configure_ui_layer(1) < 0) {
		fprintf(stderr, "warning: failed to configure UI layer\n");
	}

	p = TPlayerCreate(CEDARX_PLAYER);
	if (!p) {
		fprintf(stderr, "TPlayerCreate failed\n");
		configure_ui_layer(0);
		return 1;
	}

	if (TPlayerSetNotifyCallback(
			p,
			onPlayerNotify,
			NULL
		) != 0) {
		fprintf(stderr, "TPlayerSetNotifyCallback failed\n");
		goto fail;
	}

	printf(
		"display rect: x=%d y=%d width=%d height=%d\n",
		x,
		y,
		width,
		height
	);

	if (TPlayerSetDisplayRect(
			p,
			x,
			y,
			width,
			height
		) != 0) {
		fprintf(stderr, "TPlayerSetDisplayRect failed\n");
		goto fail;
	}

	/*
	 * 开启 CedarX 视频输出。
	 */
	if (TPlayerSetVideoDisplay(p, 1) != 0) {
		fprintf(stderr, "TPlayerSetVideoDisplay(1) failed\n");
		goto fail;
	}

	if (TPlayerSetDataSource(
			p,
			argv[1],
			NULL
		) != 0) {
		fprintf(stderr, "TPlayerSetDataSource failed\n");
		goto fail;
	}

	printf("preparing...\n");

	if (TPlayerPrepare(p) != 0) {
		fprintf(stderr, "TPlayerPrepare failed\n");
		goto fail;
	}

	printf("prepared\n");

	/*
	 * Prepare 后再设一次。
	 *
	 * CedarX 在 prepare/init video layer 的过程中可能重新配置 layer，
	 * 再设置一次比较保险。
	 */
	if (TPlayerSetDisplayRect(
			p,
			x,
			y,
			width,
			height
		) != 0) {
		fprintf(stderr, "warning: second TPlayerSetDisplayRect failed\n");
	}

	if (TPlayerSetVideoDisplay(p, 1) != 0) {
		fprintf(stderr, "warning: second TPlayerSetVideoDisplay failed\n");
	}

	if (TPlayerStart(p) != 0) {
		fprintf(stderr, "TPlayerStart failed\n");
		goto fail;
	}

	printf("started\n");

	while (!quit) {
		sleep(1);
	}

	printf("stopping...\n");

	/*
	 * 顺序：
	 *
	 * 1. 关闭视频显示
	 * 2. stop
	 * 3. destroy
	 * 4. 恢复 UI layer
	 *
	 * 尽量不要 Ctrl-C 后直接让进程消失，否则 /dev/disp 的 layer
	 * 状态容易留在一个奇怪状态。
	 */

	TPlayerSetVideoDisplay(p, 0);
	TPlayerStop(p);
	TPlayerDestroy(p);
	p = NULL;

	/*
	 * 恢复官方 UI layer 的正常不透明状态。
	 */
	if (configure_ui_layer(0) != 0) {
		fprintf(stderr, "warning: failed to restore UI layer\n");
	}

	printf("destroyed\n");

	return 0;

fail:
	if (p) {
		TPlayerSetVideoDisplay(p, 0);
		TPlayerStop(p);
		TPlayerDestroy(p);
	}

	configure_ui_layer(0);

	return 1;
}

