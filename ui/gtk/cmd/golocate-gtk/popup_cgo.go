package main

/*
#cgo pkg-config: gtk4
#include <gtk/gtk.h>
#include <gtk/gtkwidget.h>

// gotk4 v0.3 lacks Popover.SetParent. In GTK4 a GtkPopover must be a child of
// a toplevel (gtk_widget_set_parent) before gtk_popover_popup; its anchor
// rectangle is expressed in the toplevel's coordinates, converted here from
// the results-tree coordinates.
static void popover_set_parent(GtkPopover *pop, GtkWidget *toplevel) {
	gtk_widget_set_parent(GTK_WIDGET(pop), toplevel);
}
static gboolean popover_translate(GtkWidget *from, GtkWidget *to,
	gdouble x, gdouble y, gdouble *tx, gdouble *ty) {
	return gtk_widget_translate_coordinates(from, to, x, y, tx, ty);
}
*/
import "C"

import (
	"reflect"
	"unsafe"

	coreglib "github.com/diamondburned/gotk4/pkg/core/glib"
)

// rawNative returns the underlying GObject pointer of a gotk4 object (via the
// embedded *coreglib.Object; some types have ambiguous Native()).
func rawNative(o any) unsafe.Pointer {
	if o == nil {
		return nil
	}
	rv := reflect.ValueOf(o)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return nil
	}
	f := rv.Elem().FieldByName("Object")
	if !f.IsValid() || f.Kind() != reflect.Ptr || f.IsNil() {
		return nil
	}
	obj, ok := f.Interface().(*coreglib.Object)
	if !ok {
		return nil
	}
	return unsafe.Pointer(obj.Native())
}

// popoverSetParentNative anchors pop to toplevel (required before Popup()).
func popoverSetParentNative(pop, toplevel unsafe.Pointer) {
	if pop == nil || toplevel == nil {
		return
	}
	C.popover_set_parent((*C.GtkPopover)(pop), (*C.GtkWidget)(toplevel))
}

// popoverTranslateNative converts (x,y) from from's coords into to's coords.
func popoverTranslateNative(from, to unsafe.Pointer, x, y float64) (int, int, bool) {
	if from == nil || to == nil {
		return 0, 0, false
	}
	var tx, ty C.gdouble
	if C.popover_translate((*C.GtkWidget)(from), (*C.GtkWidget)(to),
		C.gdouble(x), C.gdouble(y), &tx, &ty) == 0 {
		return 0, 0, false
	}
	return int(tx), int(ty), true
}
